package actiongen

// KeyValue is an ordered key-value pair.
type KeyValue struct {
	Key   string
	Value string
}

type Input struct {
	Name        string
	Description string
	Default     string
	Required    bool
}

type Output struct {
	Name        string
	Description string
	Value       string
}

type Step struct {
	Name    string
	ID      string
	If      string
	Uses    string
	With    []KeyValue
	Shell   string
	Run     string
	Env     []KeyValue
	Comment string
}

type Branding struct {
	Icon  string
	Color string
}

type Action struct {
	Name        string
	Description string
	Author      string
	Branding    Branding
	Header      string
	Inputs      []Input
	Outputs     []Output
	Steps       []Step
}
