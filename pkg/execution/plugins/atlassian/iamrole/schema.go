package iamrole

// Schema for serviceEnvironment copied from Viceroy
// https://stash.atlassian.com/projects/MDATA/repos/viceroy/browse/src/main/resources/schemas/cloudformation.json#28
const schema = `

{
  "$schema": "http://json-schema.org/draft-04/schema#",
  "type": "object",
  "required": ["computeType", "assumeRoles", "oapResourceName", "serviceId", "serviceEnvironment"],
  "additionalProperties": false,
  "properties": {
	"computeType": {
      "type": "string",
      "enum": [
		"ec2Compute",
		"kubeCompute"
	  ]
    },
    "createInstanceProfile": {
      "type": "boolean"
    },
    "assumeRoles": {
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "managedPolicies": {
      "type": "array",
      "items": {
        "type": "string"
      }
    },
    "oapResourceName": {
      "type": "string",
      "minLength": 1
    },
    "serviceEnvironment": {
      "type": "object",
      "required": [
        "lowPriorityPagerdutyEndpoint",
        "pagerdutyEndpoint",
        "notificationEmail",
        "tags",
        "primaryVpcEnvironment"
      ],
      "additionalProperties": false,
      "properties": {
        "lowPriorityPagerdutyEndpoint": {
          "type": "string"
        },
        "pagerdutyEndpoint": {
          "type": "string"
        },
        "tags": {
          "type": "object",
          "additionalProperties": false,
          "maxProperties": 50,
          "patternProperties": {
            "^[a-zA-Z_]{1,127}$": {
              "pattern": "^[a-zA-Z0-9_ +=/.\\-:]{1,254}$",
              "type": "string"
            }
          }
        },
        "serviceSecurityGroup": {
          "type": "string"
        },
        "role": {
          "type": "string"
        },
        "roleArn": {
          "type": "string"
        },
        "notificationEmail": {
          "type": "string"
        },
        "primaryVpcEnvironment": {
          "type": "object",
          "required": [
            "vpcId",
            "privateDnsZone",
            "privatePaasDnsZone",
            "appSubnets",
            "zones"
          ],
          "additionalProperties": false,
          "properties": {
            "appSubnets": {
              "type": "array"
            },
            "vpcId": {
              "type": "string"
            },
            "jumpboxSecurityGroup": {
              "type": "string"
            },
            "instanceSecurityGroup": {
              "type": "string"
            },
            "sslCertificateId": {
              "type": "string"
            },
            "paasDnsZone": {
              "type": "string"
            },
            "paasPublicDnsZone": {
              "type": "string"
            },
            "privateDnsZone": {
              "type": "string"
            },
            "privatePaasDnsZone": {
              "type": "string"
            },
            "label": {
              "type": "string"
            },
            "region": {
              "type": "string"
            },
            "allowUserWrite": {
              "type": "boolean"
            },
            "allowUserRead": {
              "type": "boolean"
            },
            "zones": {
              "type": "array"
            }
          }
        }
      }
    },
    "serviceId": {
      "type": "string",
      "minLength": 1
    }
  }
}
`
