package actiongen

import (
	"fmt"
	"strings"
)

const HeaderGenerated = `# ARCHIVO GENERADO — no lo edites a mano.
# Lo produce actiongen y lo mantiene sincronizado un test.
# Para cambiarlo, edita action_data.go y corre: gotest ./...`

// Render produce el YAML de la action. Es determinista.
func (a Action) Render() []byte {
	var sb strings.Builder

	header := a.Header
	if header == "" {
		header = HeaderGenerated
	}

	for _, line := range strings.Split(header, "\n") {
		if strings.HasPrefix(line, "#") || line == "" {
			sb.WriteString(line)
		} else {
			sb.WriteString("# ")
			sb.WriteString(line)
		}
		sb.WriteString("\n")
	}
	sb.WriteString("\n")

	if a.Name != "" {
		fmt.Fprintf(&sb, "name: %s\n", quoteIfNeeded(a.Name))
	}
	if a.Description != "" {
		fmt.Fprintf(&sb, "description: %s\n", quoteIfNeeded(a.Description))
	}
	if a.Author != "" {
		fmt.Fprintf(&sb, "author: %s\n", quoteIfNeeded(a.Author))
	}

	if a.Branding.Icon != "" || a.Branding.Color != "" {
		sb.WriteString("branding:\n")
		if a.Branding.Icon != "" {
			fmt.Fprintf(&sb, "  icon: %s\n", quoteIfNeeded(a.Branding.Icon))
		}
		if a.Branding.Color != "" {
			fmt.Fprintf(&sb, "  color: %s\n", quoteIfNeeded(a.Branding.Color))
		}
	}

	if len(a.Inputs) > 0 {
		sb.WriteString("inputs:\n")
		for _, in := range a.Inputs {
			fmt.Fprintf(&sb, "  %s:\n", in.Name)
			if in.Description != "" {
				fmt.Fprintf(&sb, "    description: %s\n", quoteIfNeeded(in.Description))
			}
			if in.Required {
				sb.WriteString("    required: true\n")
			} else {
				sb.WriteString("    required: false\n")
			}
			if in.Default != "" {
				fmt.Fprintf(&sb, "    default: %s\n", quoteIfNeeded(in.Default))
			}
		}
	}

	if len(a.Outputs) > 0 {
		sb.WriteString("outputs:\n")
		for _, out := range a.Outputs {
			fmt.Fprintf(&sb, "  %s:\n", out.Name)
			if out.Description != "" {
				fmt.Fprintf(&sb, "    description: %s\n", quoteIfNeeded(out.Description))
			}
			if out.Value != "" {
				fmt.Fprintf(&sb, "    value: %s\n", quoteIfNeeded(out.Value))
			}
		}
	}

	sb.WriteString("runs:\n")
	sb.WriteString("  using: composite\n")
	sb.WriteString("  steps:\n")

	for _, step := range a.Steps {
		if step.Comment != "" {
			for _, line := range strings.Split(step.Comment, "\n") {
				sb.WriteString("    # ")
				sb.WriteString(line)
				sb.WriteString("\n")
			}
		}
		if step.Name != "" {
			fmt.Fprintf(&sb, "    - name: %s\n", quoteIfNeeded(step.Name))
		} else {
			sb.WriteString("    -\n")
		}

		if step.ID != "" {
			fmt.Fprintf(&sb, "      id: %s\n", quoteIfNeeded(step.ID))
		}
		if step.If != "" {
			fmt.Fprintf(&sb, "      if: %s\n", quoteIfNeeded(step.If))
		}
		if step.Uses != "" {
			fmt.Fprintf(&sb, "      uses: %s\n", quoteIfNeeded(step.Uses))
		}

		if len(step.With) > 0 {
			sb.WriteString("      with:\n")
			for _, kv := range step.With {
				if strings.Contains(kv.Value, "\n") {
					fmt.Fprintf(&sb, "        %s: |\n", kv.Key)
					for _, line := range strings.Split(kv.Value, "\n") {
						if line == "" {
							sb.WriteString("\n")
						} else {
							fmt.Fprintf(&sb, "          %s\n", line)
						}
					}
				} else {
					fmt.Fprintf(&sb, "        %s: %s\n", kv.Key, quoteIfNeeded(kv.Value))
				}
			}
		}

		if step.Shell != "" {
			fmt.Fprintf(&sb, "      shell: %s\n", quoteIfNeeded(step.Shell))
		}

		if step.Run != "" {
			if strings.Contains(step.Run, "\n") {
				sb.WriteString("      run: |\n")
				for _, line := range strings.Split(step.Run, "\n") {
					if line == "" {
						sb.WriteString("\n")
					} else {
						sb.WriteString("        ")
						sb.WriteString(line)
						sb.WriteString("\n")
					}
				}
			} else {
				fmt.Fprintf(&sb, "      run: %s\n", quoteIfNeeded(step.Run))
			}
		}

		if len(step.Env) > 0 {
			sb.WriteString("      env:\n")
			for _, kv := range step.Env {
				fmt.Fprintf(&sb, "        %s: %s\n", kv.Key, quoteIfNeeded(kv.Value))
			}
		}
	}

	res := sb.String()
	if !strings.HasSuffix(res, "\n") {
		res += "\n"
	}
	return []byte(res)
}

func quoteIfNeeded(s string) string {
	if s == "" {
		return "''"
	}
	if s == "true" || s == "false" || strings.ContainsAny(s, ":#{}[]|>&*!?'\"") || strings.HasPrefix(s, " ") || strings.HasSuffix(s, " ") {
		// Escape single quotes inside single-quoted string
		escaped := strings.ReplaceAll(s, "'", "''")
		return "'" + escaped + "'"
	}
	return s
}
