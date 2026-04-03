package commons

import databasev4 "github.com/oracle/oracle-database-operator/apis/database/v4"

func cloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]string, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func mergeStringMaps(base map[string]string, overlay map[string]string) map[string]string {
	if len(base) == 0 && len(overlay) == 0 {
		return nil
	}
	out := cloneStringMap(base)
	if out == nil {
		out = map[string]string{}
	}
	for k, v := range overlay {
		out[k] = v
	}
	return out
}

func resolveShardServiceAnnotations(instance *databasev4.ShardingDatabase, spec databasev4.ShardSpec, svcType string) map[string]string {
	if svcType == shardServiceTypeExternal {
		return cloneStringMap(spec.ExternalServiceAnnotations)
	}
	return cloneStringMap(spec.ServiceAnnotations)
}

func resolveCatalogServiceAnnotations(instance *databasev4.ShardingDatabase, spec databasev4.CatalogSpec, svcType string) map[string]string {
	if svcType == shardServiceTypeExternal {
		return cloneStringMap(spec.ExternalServiceAnnotations)
	}
	return cloneStringMap(spec.ServiceAnnotations)
}

func resolveGsmServiceAnnotations(instance *databasev4.ShardingDatabase, spec databasev4.GsmSpec, svcType string) map[string]string {
	if svcType == shardServiceTypeExternal {
		base := mergeStringMaps(instance.Spec.GsmExternalServiceAnnotations, gsmInfoExternalServiceAnnotations(instance))
		return mergeStringMaps(base, spec.ExternalServiceAnnotations)
	}
	base := mergeStringMaps(instance.Spec.GsmServiceAnnotations, gsmInfoServiceAnnotations(instance))
	return mergeStringMaps(base, spec.ServiceAnnotations)
}

func gsmInfoServiceAnnotations(instance *databasev4.ShardingDatabase) map[string]string {
	if instance == nil || instance.Spec.GsmInfo == nil {
		return nil
	}
	return instance.Spec.GsmInfo.ServiceAnnotations
}

func gsmInfoExternalServiceAnnotations(instance *databasev4.ShardingDatabase) map[string]string {
	if instance == nil || instance.Spec.GsmInfo == nil {
		return nil
	}
	return instance.Spec.GsmInfo.ExternalServiceAnnotations
}
