package goflare

func (h *Goflare) Name() string { return "GOFLARE" }
func (h *Goflare) Label() string {
	return "Build Workers"

}
func (h *Goflare) Value() string {
	return ""
}
func (h *Goflare) Change(newValue string, progress func(msgs ...any)) {
	var err error

	switch newValue {
	case "f": // Pages build shortcut
		if progress != nil {
			progress("Starting Pages build...")
		}
		err = h.GeneratePagesFiles()
		if err != nil {
			if progress != nil {
				progress("Pages build failed:", err.Error())
			}
			return
		}
		if progress != nil {
			progress("Pages build completed successfully")
		}

	case "w": // Workers build shortcut
		if progress != nil {
			progress("Starting Workers build...")
		}
		err = h.GenerateWorkerFiles()
		if err != nil {
			if progress != nil {
				progress("Workers build failed:", err.Error())
			}
			return
		}
		if progress != nil {
			progress("Workers build completed successfully")
		}

	default:
		if progress != nil {
			progress("Unknown shortcut:", newValue)
		}
	}
}

func (h *Goflare) Shortcuts() []map[string]string {
	return []map[string]string{
		{"f": "Build Cloudflare Functions Pages Files"},
		{"w": "Build Cloudflare Workers Files"},
	}
}
