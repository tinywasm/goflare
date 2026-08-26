//go:build !wasm

package goflare

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"os"
	"strconv"
)

const (
	// WasmWarnSizeKiB es el umbral de aviso sobre el tamaño CRUDO.
	WasmWarnSizeKiB = 256

	// WasmMaxSizeKiB es el presupuesto PROPIO de goflare, no un limite de Cloudflare.
	WasmMaxSizeKiB = 900

	// EnvKeyWasmWarnSizeKiB permite subir o bajar el aviso sin recompilar.
	EnvKeyWasmWarnSizeKiB = "WASM_WARN_SIZE_KIB"

	// EnvKeyWasmMaxSizeKiB permite subir o bajar el corte duro sin recompilar.
	EnvKeyWasmMaxSizeKiB = "WASM_MAX_SIZE_KIB"

	// WasmArtifactName es el nombre estandar del binario WASM del Worker.
	WasmArtifactName = "edge.wasm"

	bytesPerKiB = 1024
)

const (
	wasmSizeReportFmt = "%s: %d B crudo (%.1f KiB) | %d B gzip (%.1f KiB)"
	wasmWarnMsgFmt    = "warning: %s pesa %.1f KiB crudo, sobre el umbral de aviso de %.1f KiB — cada KiB se paga en tiempo de arranque del isolate (Cloudflare da 1 s). El limite duro de Cloudflare es sobre el gzip: 3 MB en Free, 10 MB en Paid."
	wasmErrMsgFmt     = "%s pesa %.1f KiB crudo, sobre el presupuesto de %.1f KiB que impone goflare — despliegue detenido antes de gastar la subida. Este presupuesto es de goflare, no de Cloudflare (cuyo limite es 3 MB gzip en Free). Reduce el binario o sube WASM_MAX_SIZE_KIB."
)

// WasmSizes son las dos magnitudes que importan de un artefacto del edge: la
// cruda, que es la que el isolate compila e instancia, y la comprimida, que es
// la que Cloudflare pesa contra su limite.
type WasmSizes struct {
	Raw  int64
	Gzip int64
}

// MeasureWasm devuelve el tamaño crudo y el comprimido con gzip del archivo.
func MeasureWasm(path string) (WasmSizes, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return WasmSizes{}, fmt.Errorf("measure wasm read file: %w", err)
	}

	rawSize := int64(len(data))

	var buf bytes.Buffer
	gw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return WasmSizes{}, fmt.Errorf("measure wasm gzip init: %w", err)
	}

	if _, err := gw.Write(data); err != nil {
		gw.Close()
		return WasmSizes{}, fmt.Errorf("measure wasm gzip write: %w", err)
	}

	if err := gw.Close(); err != nil {
		return WasmSizes{}, fmt.Errorf("measure wasm gzip close: %w", err)
	}

	return WasmSizes{
		Raw:  rawSize,
		Gzip: int64(buf.Len()),
	}, nil
}

func wasmWarnSizeBytes() int64 {
	if val := os.Getenv(EnvKeyWasmWarnSizeKiB); val != "" {
		if kib, err := strconv.ParseInt(val, 10, 64); err == nil && kib >= 0 {
			return kib * bytesPerKiB
		}
	}
	return WasmWarnSizeKiB * bytesPerKiB
}

func wasmMaxSizeBytes() int64 {
	if val := os.Getenv(EnvKeyWasmMaxSizeKiB); val != "" {
		if kib, err := strconv.ParseInt(val, 10, 64); err == nil && kib >= 0 {
			return kib * bytesPerKiB
		}
	}
	return WasmMaxSizeKiB * bytesPerKiB
}

// CheckWasmSize mide el artefacto, escribe SIEMPRE el reporte en log, avisa si
// supera el umbral de aviso, y devuelve error si supera el presupuesto.
func CheckWasmSize(path string, log func(...any)) error {
	sizes, err := MeasureWasm(path)
	if err != nil {
		return err
	}

	if log == nil {
		log = func(...any) {}
	}

	rawKiB := float64(sizes.Raw) / bytesPerKiB
	gzipKiB := float64(sizes.Gzip) / bytesPerKiB

	log(fmt.Sprintf(wasmSizeReportFmt, WasmArtifactName, sizes.Raw, rawKiB, sizes.Gzip, gzipKiB))

	warnLimitBytes := wasmWarnSizeBytes()
	if warnLimitBytes > 0 && sizes.Raw >= warnLimitBytes {
		warnLimitKiB := float64(warnLimitBytes) / bytesPerKiB
		log(fmt.Sprintf(wasmWarnMsgFmt, WasmArtifactName, rawKiB, warnLimitKiB))
	}

	maxLimitBytes := wasmMaxSizeBytes()
	if maxLimitBytes > 0 && sizes.Raw > maxLimitBytes {
		maxLimitKiB := float64(maxLimitBytes) / bytesPerKiB
		return fmt.Errorf(wasmErrMsgFmt, WasmArtifactName, rawKiB, maxLimitKiB)
	}

	return nil
}
