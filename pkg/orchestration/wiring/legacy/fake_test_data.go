package legacy

import "github.com/atlassian/voyager"

const (
	TestAccountName voyager.Account = "testaccount"
	TestEnvironment voyager.EnvType = "testenv"
	TestRegion      voyager.Region  = "testregion"
)

var (
	TestLegacyConfigs = map[voyager.EnvType]map[voyager.Region]map[voyager.Account]Config{
		TestEnvironment: {
			TestRegion: {
				TestAccountName: Config{
					MicrosEnv:             "microstestenv",
					ManagedPolicies:       []string{"arn:aws:iam::123456789012:policy/SOX-DENY-IAM-CREATE-DELETE", "arn:aws:iam::123456789012:policy/micros-iam-DefaultServicePolicy-ABC"},
					CertificateID:         "arn:aws:acm:testregion:123456789012:certificate/253b42fa-047c-44c2-8bac-777777777777",
					InternalLbSubnets:     []string{"subnet-1", "subnet-2"},
					AppSubnets:            []string{"subnet-1", "subnet-2"},
					PrivilegedAppSubnets:  []string{"subnet-3", "subnet-4"},
					NoSCAppSubnets:        []string{"subnet-5", "subnet-6"},
					LambdaSubnets:         []string{"subnet-1", "subnet-2"},
					AutoScalingSnsArn:     "arn:aws:sns:testregion:123456789012:micros-auto-scaling-sns-testenv",
					LambdaBucket:          "lambda-artifact-store.testregion.atl-inf.io",
					AliasZone:             "test.atlassian.io",
					AllowUserWrite:        false,
					AllowUserRead:         true,
					Vpc:                   "vpc-1",
					Private:               "testregion.atl-inf.io",
					PrivatePaas:           "testregion.dev.paas-inf.net",
					DNS:                   "testregion.dev.atl-paas.net",
					PublicDNS:             "testregion.dev.public.atl-paas.net",
					LegacyDNS:             "testregion.atlassian.io",
					Zones:                 []string{"testregiona", "testregionb"},
					JumpboxSecurityGroup:  "sg-1",
					InstanceSecurityGroup: "sg-2",
					LambdaSecurityGroup:   "sg-3",
					LbSubnets:             []string{"subnet-7", "subnet-8"},
					LbSecurityGroup:       "sg-4",
					Certificate:           "arn:aws:acm:testregion:123456789012:certificate/e6d6eef4-76a4-4c1d-92c0-777777777777",
					EMRSubnet:             "subnet-1a",
					DeployerRole:          "arn:aws:iam::123456789012:role/micros-server-iam-MicrosServer-ABC",
				},
			},
		},
	}
)
