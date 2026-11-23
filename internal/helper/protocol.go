package helper

type Request struct {
	Action        string   `json:"action"`
	Domain        string   `json:"domain,omitempty"`
	DocumentRoot  string   `json:"document_root,omitempty"`
	ServerAliases []string `json:"server_aliases,omitempty"`
	BackupID      string   `json:"backup_id,omitempty"`
}

type Response struct {
	Success bool   `json:"success"`
	Output  string `json:"output,omitempty"`
	Error   string `json:"error,omitempty"`
}
