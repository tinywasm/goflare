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
	// WasmWarnSizeKiB is the warning threshold, measured on the RAW size.
	WasmWarnSizeKiB = 256

	// WasmMaxSizeKiB is goflare's OWN budget, not a Cloudflare limit.
	WasmMaxSizeKiB = 900

	// EnvKeyWasmWarnSizeKiB raises or lowers the warning without a rebuild.
	EnvKeyWasmWarnSizeKiB = "WASM_WARN_SIZE_KIB"

	// EnvKeyWasmMaxSizeKiB raises or lowers the hard cut-off without a rebuild.
	EnvKeyWasmMaxSizeKiB = "WASM_MAX_SIZE_KIB"

	// WasmArtifactName is the standard name of the Worker's WASM binary.
	WasmArtifactName = "edge.wasm"

	bytesPerKiB = 1024
)

const (
	wasmSizeReportFmt = "%s: %d B raw (%.1f KiB) | %d B gzip (%.1f KiB)"
	wasmWarnMsgFmt    = "warning: %s weighs %.1f KiB raw, over the %.1f KiB warning threshold — every KiB is paid in isolate startup time (Cloudflare allows 1 s). Cloudflare's hard limit is on the gzip size: 3 MB on Free, 10 MB on Paid."
	wasmErrMsgFmt     = "%s weighs %.1f KiB raw, over the %.1f KiB budget goflare imposes — deploy stopped before spending the upload. This budget is goflare's, not Cloudflare's (whose limit is 3 MB gzip on Free). Shrink the binary or raise WASM_MAX_SIZE_KIB."
)

// WasmSizes holds the two figures that matter for an edge artifact: the raw
// one, which is what the isolate compiles and instantiates, and the compressed
// one, which is what Cloudflare weighs against its limit.
type WasmSizes struct {
	Raw  int64
	Gzip int64
}

// MeasureWasm returns the file's raw size and its gzip-compressed size.
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

// CheckWasmSize measures the artifact, ALWAYS writes the report to log, warns
// when it passes the warning threshold, and errors when it passes the budget.
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
