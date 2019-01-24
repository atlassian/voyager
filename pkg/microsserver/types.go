package microsserver

type AliasInfo struct {
	Alias   Alias   `json:"alias"`
	Service Service `json:"service"`
}

type Alias struct {
	DomainName string `json:"domainName"`
}

type Service struct {
	Name  string `json:"name"`
	Owner string `json:"owner"`
}
