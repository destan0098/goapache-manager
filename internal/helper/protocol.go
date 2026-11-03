package helper

type Request struct {
	Action string `json:"action"`
	Domain string `json:"domain,omitempty"`
}

type Response struct {
	Success bool   `json:"success"`
	Output  string `json:"output,omitempty"`
	Error   string `json:"error,omitempty"`
}
