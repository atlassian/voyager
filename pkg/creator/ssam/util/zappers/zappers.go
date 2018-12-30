package zappers

import (
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func ContainerShortName(name string) zapcore.Field {
	return zap.String("container_short_name", name)
}

func ContainerSystemOwner(owner string) zapcore.Field {
	return zap.String("container_system_owner", owner)
}

func ServiceName(name string) zapcore.Field {
	return zap.String("service_name", name)
}

func ServiceOwner(owner string) zapcore.Field {
	return zap.String("service_owner", owner)
}

func AccessLevelShortName(name string) zapcore.Field {
	return zap.String("access_level_short_name", name)
}

func AccessLevelMembers(users []string) zapcore.Field {
	return zap.String("access_level_members_users", strings.Join(users, ", "))
}

func ASAPKeyID(keyID string) zapcore.Field {
	return zap.String("asap_key_id", keyID)
}

func ASAPKeyIssuer(issuer string) zapcore.Field {
	return zap.String("asap_key_issuer", issuer)
}
