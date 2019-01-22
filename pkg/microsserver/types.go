package microsserver

type AliasInfo struct {
	Service Service `json:"Service"`
}

type Service struct {
	Name  string `json:"name"`
	Owner string `json:"owner"`
}
