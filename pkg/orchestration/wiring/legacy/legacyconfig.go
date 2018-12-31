package legacy

import "github.com/atlassian/voyager"

type Config struct {
	MicrosEnv             string
	ManagedPolicies       []string
	CertificateID         string
	InternalLbSubnets     []string
	AppSubnets            []string
	PrivilegedAppSubnets  []string
	NoSCAppSubnets        []string
	LambdaSubnets         []string
	AutoScalingSnsArn     string
	LambdaBucket          string
	AliasZone             string
	AllowUserRead         bool
	AllowUserWrite        bool
	Vpc                   string
	Private               string
	PrivatePaas           string
	DNS                   string
	PublicDNS             string
	LegacyDNS             string
	Zones                 []string
	JumpboxSecurityGroup  string
	InstanceSecurityGroup string
	LambdaSecurityGroup   string
	LbSubnets             []string
	LbSecurityGroup       string
	Certificate           string
	EMRSubnet             string
	DeployerRole          string
}

// Extract the config from map; return nil if it doesn't exist
func GetLegacyConfigFromMap(legacyConfigMap map[voyager.EnvType]map[voyager.Region]map[voyager.Account]Config, location voyager.Location) *Config {
	regionMap := legacyConfigMap[location.EnvType]
	if regionMap == nil {
		return nil
	}
	accountMap := regionMap[location.Region]
	if accountMap == nil {
		return nil
	}
	if config, ok := accountMap[location.Account]; ok {
		return &config
	}
	return nil
}
