package v4

import "testing"

func TestEffectivePrivateAiSpecFromSpec_PrefersHierarchicalFields(t *testing.T) {
	trueValue := true
	falseValue := false
	replicas := int32(3)
	httpsPort := int32(9443)
	sizeInGb := int32(120)

	spec := &PrivateAiSpec{
		PaiEnableAuthentication: "false",
		PaiSecret: &PaiSecretSpec{
			Name:          "legacy-auth",
			MountLocation: "/legacy/auth",
		},
		PaiConfigFile: &PaiConfigMap{
			Name:          "legacy-config",
			MountLocation: "/legacy/config",
		},
		PaiImage:           "legacy-image",
		PaiImagePullSecret: "legacy-pull-secret",
		Replicas:           1,
		StorageClass:       "legacy-storage",
		StorageSizeInGb:    10,
		PaiHTTPEnabled:     "true",
		PaiHTTPPort:        8080,
		TrafficManager: &TrafficManagerRefSpec{
			Ref:       "legacy-tm",
			RoutePath: "/legacy/v1/",
		},
		Security: &PrivateAiSecuritySpec{
			AuthEnabled: &trueValue,
			Secret: &PaiSecretSpec{
				Name:          "new-auth",
				MountLocation: "/run/secrets",
			},
		},
		Runtime: &PrivateAiRuntimeSpec{
			Image: &PrivateAiImageSpec{
				Name:       "new-image",
				PullSecret: "new-pull-secret",
			},
			Replicas: &replicas,
		},
		Configuration: &PrivateAiConfigurationSpec{
			ConfigFile: &PaiConfigMap{
				Name:          "new-config",
				MountLocation: "/privateai/config",
			},
		},
		Storage: &PrivateAiStorageSpec{
			StorageClass: "new-storage",
			SizeInGb:     &sizeInGb,
		},
		Networking: &PrivateAiNetworkingSpec{
			Listeners: &PrivateAiListenersSpec{
				HTTP:  &PrivateAiListenerSpec{Enabled: &falseValue},
				HTTPS: &PrivateAiListenerSpec{Enabled: &trueValue, Port: &httpsPort},
			},
			TrafficManager: &TrafficManagerRefSpec{
				Ref:       "new-tm",
				RoutePath: "/new/v1/",
			},
		},
	}

	effective := EffectivePrivateAiSpecFromSpec(spec)

	if !effective.Security.AuthEnabled {
		t.Fatalf("expected hierarchical security.authEnabled to win")
	}
	if effective.Security.Secret == nil || effective.Security.Secret.Name != "new-auth" {
		t.Fatalf("expected hierarchical auth secret, got %#v", effective.Security.Secret)
	}
	if effective.Configuration.ConfigFile == nil || effective.Configuration.ConfigFile.Name != "new-config" {
		t.Fatalf("expected hierarchical config file, got %#v", effective.Configuration.ConfigFile)
	}
	if effective.Runtime.Image == nil || effective.Runtime.Image.Name != "new-image" {
		t.Fatalf("expected hierarchical image, got %#v", effective.Runtime.Image)
	}
	if effective.Runtime.Replicas != 3 {
		t.Fatalf("expected hierarchical replicas 3, got %d", effective.Runtime.Replicas)
	}
	if effective.Storage.StorageClass != "new-storage" {
		t.Fatalf("expected hierarchical storage class, got %q", effective.Storage.StorageClass)
	}
	if effective.Storage.SizeInGb != 120 {
		t.Fatalf("expected hierarchical size 120, got %d", effective.Storage.SizeInGb)
	}
	if effective.Networking.Listeners.HTTP.Enabled {
		t.Fatalf("expected hierarchical HTTP disabled")
	}
	if !effective.Networking.Listeners.HTTPS.Enabled || effective.Networking.Listeners.HTTPS.Port != 9443 {
		t.Fatalf("expected hierarchical HTTPS 9443, got %+v", effective.Networking.Listeners.HTTPS)
	}
	if effective.Networking.TrafficManager == nil || effective.Networking.TrafficManager.Ref != "new-tm" {
		t.Fatalf("expected hierarchical traffic manager, got %#v", effective.Networking.TrafficManager)
	}
}

func TestEffectivePrivateAiSpecFromSpec_FallsBackToLegacyFields(t *testing.T) {
	spec := &PrivateAiSpec{
		PaiEnableAuthentication: "true",
		PaiSecret: &PaiSecretSpec{
			Name:          "legacy-auth",
			MountLocation: "/legacy/auth",
		},
		PaiConfigFile: &PaiConfigMap{
			Name:          "legacy-config",
			MountLocation: "/legacy/config",
		},
		PaiImage:           "legacy-image",
		PaiImagePullSecret: "legacy-pull-secret",
		Replicas:           2,
		StorageClass:       "legacy-storage",
		StorageSizeInGb:    50,
		PaiHTTPEnabled:     "false",
		PaiHTTPSEnabled:    "true",
		PaiHTTPSPort:       8443,
	}

	effective := EffectivePrivateAiSpecFromSpec(spec)

	if !effective.Security.AuthEnabled {
		t.Fatalf("expected legacy auth enabled to be preserved")
	}
	if effective.Security.Secret == nil || effective.Security.Secret.Name != "legacy-auth" {
		t.Fatalf("expected legacy auth secret, got %#v", effective.Security.Secret)
	}
	if effective.Configuration.ConfigFile == nil || effective.Configuration.ConfigFile.Name != "legacy-config" {
		t.Fatalf("expected legacy config file, got %#v", effective.Configuration.ConfigFile)
	}
	if effective.Runtime.Image == nil || effective.Runtime.Image.Name != "legacy-image" || effective.Runtime.Image.PullSecret != "legacy-pull-secret" {
		t.Fatalf("expected legacy image settings, got %#v", effective.Runtime.Image)
	}
	if effective.Runtime.Replicas != 2 {
		t.Fatalf("expected legacy replicas 2, got %d", effective.Runtime.Replicas)
	}
	if effective.Storage.StorageClass != "legacy-storage" || effective.Storage.SizeInGb != 50 {
		t.Fatalf("expected legacy storage settings, got %+v", effective.Storage)
	}
	if effective.Networking.Listeners.HTTP.Enabled {
		t.Fatalf("expected legacy HTTP disabled")
	}
	if !effective.Networking.Listeners.HTTPS.Enabled || effective.Networking.Listeners.HTTPS.Port != 8443 {
		t.Fatalf("expected legacy HTTPS 8443, got %+v", effective.Networking.Listeners.HTTPS)
	}
}

func TestEffectivePrivateAiSpecFromSpec_FallsBackToLegacyPortMappings(t *testing.T) {
	spec := &PrivateAiSpec{
		PaiService: PaiServiceSpec{
			External: &PaiExternalServiceSpec{
				LoadBalancerIP: "1.2.3.4",
			},
		},
		PortMappings: []PaiPortMapping{
			{
				Port:       443,
				TargetPort: 8443,
			},
		},
	}

	effective := EffectivePrivateAiSpecFromSpec(spec)

	if effective.Networking.Service == nil {
		t.Fatalf("expected effective service to be present")
	}
	if len(effective.Networking.Service.PortMappings) != 1 {
		t.Fatalf("expected legacy top-level portMappings to be preserved, got %#v", effective.Networking.Service.PortMappings)
	}
	if effective.Networking.Service.PortMappings[0].Port != 443 || effective.Networking.Service.PortMappings[0].TargetPort != 8443 {
		t.Fatalf("unexpected effective port mapping: %#v", effective.Networking.Service.PortMappings[0])
	}
	if effective.Networking.Service.External == nil || effective.Networking.Service.External.LoadBalancerIP != "1.2.3.4" {
		t.Fatalf("expected legacy paiService.external to be preserved, got %#v", effective.Networking.Service.External)
	}
}
