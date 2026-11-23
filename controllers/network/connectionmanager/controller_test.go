package connectionmanager

import (
	"testing"

	networkv4 "github.com/oracle/oracle-database-operator/apis/network/v4"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func TestBuildConnectionManagerEnvVarsFileMode(t *testing.T) {
	inst := &networkv4.ConnectionManager{
		ObjectMeta: metav1.ObjectMeta{Name: "cman", Namespace: "net"},
		Spec: networkv4.ConnectionManagerSpec{
			ConfigSource: &networkv4.ConnectionManagerConfigSourceSpec{
				ConfigMapRef: &networkv4.ConnectionManagerConfigMapKeyRef{Name: "cman-config", Key: "cman.ora"},
			},
			LogLevel:                 "admin",
			TraceLevel:               "support",
			RegistrationInvitedNodes: "db1",
			RestAPI:                  &networkv4.ConnectionManagerRestAPISpec{Enabled: true},
		},
	}

	envs := buildConnectionManagerEnvVars(inst)
	got := map[string]string{}
	for _, env := range envs {
		got[env.Name] = env.Value
	}
	if got["USER_CMAN_FILE"] != connectionManagerUserConfigFilePath {
		t.Fatalf("expected USER_CMAN_FILE %q, got %q", connectionManagerUserConfigFilePath, got["USER_CMAN_FILE"])
	}
	if got["PUBLIC_HOSTNAME"] != "cman" || got["DOMAIN"] != "net.svc.cluster.local" {
		t.Fatalf("unexpected base CMAN environment: %#v", got)
	}
	if got["PORT"] != "1521" || got["REST_PORT"] != "1525" {
		t.Fatalf("expected default CMAN ports in file mode envs, got %#v", got)
	}
	for _, forbidden := range []string{"LOG_LEVEL", "TRACE_LEVEL", "REGISTRATION_INVITED_NODES", "REST_HOST", "ENABLE_REST_API"} {
		if _, ok := got[forbidden]; ok {
			t.Fatalf("did not expect %s in file mode envs", forbidden)
		}
	}
}

func TestBuildConnectionManagerServiceAddsRestPort(t *testing.T) {
	inst := &networkv4.ConnectionManager{
		ObjectMeta: metav1.ObjectMeta{Name: "cman", Namespace: "net"},
		Spec: networkv4.ConnectionManagerSpec{
			RestAPI: &networkv4.ConnectionManagerRestAPISpec{Enabled: true},
			Service: networkv4.ConnectionManagerServiceSpec{
				External: networkv4.ConnectionManagerServiceEndpointSpec{Enabled: ptr.To(true)},
			},
		},
	}

	svc := buildConnectionManagerService(inst, "external")
	if svc.Spec.Type != corev1.ServiceTypeLoadBalancer {
		t.Fatalf("expected external CMAN service type LoadBalancer, got %s", svc.Spec.Type)
	}
	if len(svc.Spec.Ports) != 2 {
		t.Fatalf("expected 2 service ports, got %d", len(svc.Spec.Ports))
	}
	if svc.Spec.Ports[0].Name != "cman" || svc.Spec.Ports[0].Port != 1521 {
		t.Fatalf("unexpected primary CMAN service port: %#v", svc.Spec.Ports[0])
	}
	if svc.Spec.Ports[1].Name != "rest" || svc.Spec.Ports[1].Port != 1525 {
		t.Fatalf("unexpected REST service port: %#v", svc.Spec.Ports[1])
	}
}

func TestBuildConnectionManagerDeploymentUsesOwnLabelsAndSecurity(t *testing.T) {
	inst := &networkv4.ConnectionManager{
		ObjectMeta: metav1.ObjectMeta{Name: "cman", Namespace: "net"},
		Spec: networkv4.ConnectionManagerSpec{
			Runtime: networkv4.ConnectionManagerRuntimeSpec{
				Image:                    "oracle/cman:23.8.0",
				ServiceAccountName:       "cman-sa",
				PodSecurityContext:       &corev1.PodSecurityContext{FSGroup: ptr.To[int64](54321)},
				ContainerSecurityContext: &corev1.SecurityContext{AllowPrivilegeEscalation: ptr.To(false)},
			},
			ConfigSource: &networkv4.ConnectionManagerConfigSourceSpec{
				ConfigMapRef: &networkv4.ConnectionManagerConfigMapKeyRef{Name: "cman-config", Key: "cman.ora"},
			},
		},
	}

	deploy := buildConnectionManagerDeployment(inst, "cfg-hash", "")
	if got := deploy.Labels["app.kubernetes.io/component"]; got != "connection-manager" {
		t.Fatalf("expected connection-manager component label, got %q", got)
	}
	if deploy.Spec.Template.Spec.ServiceAccountName != "cman-sa" {
		t.Fatalf("expected service account cman-sa, got %q", deploy.Spec.Template.Spec.ServiceAccountName)
	}
	if deploy.Spec.Template.Spec.SecurityContext == nil || ptr.Deref(deploy.Spec.Template.Spec.SecurityContext.FSGroup, 0) != 54321 {
		t.Fatalf("expected pod security context fsGroup 54321")
	}
	container := deploy.Spec.Template.Spec.Containers[0]
	if container.SecurityContext == nil || ptr.Deref(container.SecurityContext.AllowPrivilegeEscalation, true) {
		t.Fatalf("expected container security context allowPrivilegeEscalation=false")
	}
	if len(deploy.Spec.Template.Spec.Volumes) != 1 || deploy.Spec.Template.Spec.Volumes[0].Name != "cman-user-config" {
		t.Fatalf("expected CMAN user config volume, got %#v", deploy.Spec.Template.Spec.Volumes)
	}
}

func TestBuildConnectionManagerDeploymentUsesCustomStartupScriptAndMultiFileConfig(t *testing.T) {
	inst := &networkv4.ConnectionManager{
		ObjectMeta: metav1.ObjectMeta{Name: "cman", Namespace: "net"},
		Spec: networkv4.ConnectionManagerSpec{
			Runtime: networkv4.ConnectionManagerRuntimeSpec{
				Image: "oracle/cman:23.8.0",
				StartupScript: &networkv4.ConnectionManagerStartupScriptSpec{
					ConfigMapRef: &networkv4.ConnectionManagerConfigMapKeyRef{Name: "cman-startup", Key: "launch-prod.sh"},
				},
			},
			ConfigSource: &networkv4.ConnectionManagerConfigSourceSpec{
				ConfigMapRef: &networkv4.ConnectionManagerConfigMapKeyRef{Name: "cman-config", Key: "custom-config.conf"},
				MountPath:    "/opt/oracle/cman-config",
			},
		},
	}

	deploy := buildConnectionManagerDeployment(inst, "cfg-hash", "")
	container := deploy.Spec.Template.Spec.Containers[0]
	if len(container.Command) != 1 || container.Command[0] != "/opt/oracle/cman-scripts/start-cman.sh" {
		t.Fatalf("expected fixed startup command, got %#v", container.Command)
	}
	envs := map[string]string{}
	for _, env := range container.Env {
		envs[env.Name] = env.Value
	}
	if envs["CMAN_CONFIG_DIR"] != "/opt/oracle/cman-config" {
		t.Fatalf("expected CMAN_CONFIG_DIR, got %#v", envs)
	}
	if envs["USER_CMAN_FILE"] != "/opt/oracle/cman-config/custom-config.conf" {
		t.Fatalf("expected USER_CMAN_FILE for the selected multi-file key, got %#v", envs)
	}

	volumes := map[string]corev1.Volume{}
	for _, volume := range deploy.Spec.Template.Spec.Volumes {
		volumes[volume.Name] = volume
	}
	startupVolume := volumes["cman-startup-script"]
	if startupVolume.ConfigMap == nil || ptr.Deref(startupVolume.ConfigMap.DefaultMode, 0) != 0555 {
		t.Fatalf("expected executable startup ConfigMap volume, got %#v", startupVolume)
	}
	if len(startupVolume.ConfigMap.Items) != 1 || startupVolume.ConfigMap.Items[0].Key != "launch-prod.sh" || startupVolume.ConfigMap.Items[0].Path != "start-cman.sh" {
		t.Fatalf("expected ConfigMap key to map to fixed script filename, got %#v", startupVolume.ConfigMap.Items)
	}
	configVolume := volumes["cman-config-source"]
	if configVolume.ConfigMap == nil || configVolume.ConfigMap.Name != "cman-config" {
		t.Fatalf("expected multi-file ConfigMap volume, got %#v", configVolume)
	}
	if ptr.Deref(configVolume.ConfigMap.DefaultMode, 0) != 0444 || len(configVolume.ConfigMap.Items) != 0 {
		t.Fatalf("expected all config keys mounted read-only with default mode 0444, got %#v", configVolume.ConfigMap)
	}
	mounts := map[string]corev1.VolumeMount{}
	for _, mount := range container.VolumeMounts {
		mounts[mount.Name] = mount
	}
	if mounts["cman-startup-script"].MountPath != "/opt/oracle/cman-scripts" || !mounts["cman-startup-script"].ReadOnly {
		t.Fatalf("unexpected startup script mount: %#v", mounts["cman-startup-script"])
	}
	if mounts["cman-config-source"].MountPath != "/opt/oracle/cman-config" || !mounts["cman-config-source"].ReadOnly {
		t.Fatalf("unexpected multi-file config mount: %#v", mounts["cman-config-source"])
	}
}

func TestBuildConnectionManagerDeploymentUsesStoragePVC(t *testing.T) {
	inst := &networkv4.ConnectionManager{
		ObjectMeta: metav1.ObjectMeta{Name: "cman", Namespace: "net"},
		Spec: networkv4.ConnectionManagerSpec{
			Runtime: networkv4.ConnectionManagerRuntimeSpec{Image: "oracle/cman:23.8.0"},
			Storage: &networkv4.ConnectionManagerStorageSpec{
				MountPath: "/var/lib/cman",
				PvcName:   "precreated-cman-pvc",
			},
		},
	}

	deploy := buildConnectionManagerDeployment(inst, "cfg-hash", "")
	container := deploy.Spec.Template.Spec.Containers[0]
	if len(container.VolumeMounts) != 1 || container.VolumeMounts[0].Name != connectionManagerStorageVolumeName {
		t.Fatalf("expected CMAN storage volume mount, got %#v", container.VolumeMounts)
	}
	if container.VolumeMounts[0].MountPath != "/var/lib/cman" {
		t.Fatalf("expected CMAN storage mount path, got %q", container.VolumeMounts[0].MountPath)
	}
	if len(deploy.Spec.Template.Spec.Volumes) != 1 {
		t.Fatalf("expected one CMAN storage volume, got %#v", deploy.Spec.Template.Spec.Volumes)
	}
	volume := deploy.Spec.Template.Spec.Volumes[0]
	if volume.PersistentVolumeClaim == nil || volume.PersistentVolumeClaim.ClaimName != "precreated-cman-pvc" {
		t.Fatalf("expected existing CMAN PVC volume, got %#v", volume)
	}
}

func TestConnectionManagerStorageClaimName(t *testing.T) {
	managed := &networkv4.ConnectionManager{
		ObjectMeta: metav1.ObjectMeta{Name: "cman", Namespace: "net"},
		Spec:       networkv4.ConnectionManagerSpec{Storage: &networkv4.ConnectionManagerStorageSpec{MountPath: "/var/lib/cman", StorageSizeInGb: 10, StorageClass: "oci-bv"}},
	}
	if got := connectionManagerStorageClaimName(managed); got != "cman-storage" {
		t.Fatalf("expected generated storage PVC name cman-storage, got %q", got)
	}
}

func TestBuildConnectionManagerEnvVarsUsesConfiguredPorts(t *testing.T) {
	inst := &networkv4.ConnectionManager{
		ObjectMeta: metav1.ObjectMeta{Name: "cman", Namespace: "net"},
		Spec: networkv4.ConnectionManagerSpec{
			Runtime: networkv4.ConnectionManagerRuntimeSpec{Image: "oracle/cman:23.8.0", Port: 1621},
			RestAPI: &networkv4.ConnectionManagerRestAPISpec{Port: 1625},
		},
	}
	envs := map[string]string{}
	for _, env := range buildConnectionManagerEnvVars(inst) {
		envs[env.Name] = env.Value
	}
	if envs["PORT"] != "1621" || envs["REST_PORT"] != "1625" {
		t.Fatalf("expected configured CMAN ports, got %#v", envs)
	}
}

func TestBuildConnectionManagerDeploymentUsesPortProbes(t *testing.T) {
	inst := &networkv4.ConnectionManager{
		ObjectMeta: metav1.ObjectMeta{Name: "cman", Namespace: "net"},
		Spec: networkv4.ConnectionManagerSpec{
			Runtime: networkv4.ConnectionManagerRuntimeSpec{Image: "oracle/cman:23.8.0", Port: 1621},
		},
	}
	container := buildConnectionManagerDeployment(inst, "cfg-hash", "").Spec.Template.Spec.Containers[0]
	if container.ReadinessProbe == nil || container.ReadinessProbe.TCPSocket == nil || container.ReadinessProbe.TCPSocket.Port.IntValue() != 1621 {
		t.Fatalf("expected readiness TCP probe on port 1621, got %#v", container.ReadinessProbe)
	}
	if container.LivenessProbe == nil || container.LivenessProbe.TCPSocket == nil || container.LivenessProbe.TCPSocket.Port.IntValue() != 1621 {
		t.Fatalf("expected liveness TCP probe on port 1621, got %#v", container.LivenessProbe)
	}
}

func TestBuildConnectionManagerDeploymentUsesMaintenanceMode(t *testing.T) {
	inst := &networkv4.ConnectionManager{
		ObjectMeta: metav1.ObjectMeta{Name: "cman", Namespace: "net"},
		Spec: networkv4.ConnectionManagerSpec{
			Runtime:     networkv4.ConnectionManagerRuntimeSpec{Image: "oracle/cman:23.8.0"},
			Maintenance: &networkv4.ConnectionManagerMaintenanceSpec{Enabled: true},
		},
	}

	deployment := buildConnectionManagerDeployment(inst, "cfg-hash", "")
	container := deployment.Spec.Template.Spec.Containers[0]
	if len(container.Command) != 1 || container.Command[0] != connectionManagerMaintenanceShell {
		t.Fatalf("expected maintenance shell command, got %#v", container.Command)
	}
	if len(container.Args) != 2 || container.Args[0] != "-c" || container.Args[1] != connectionManagerMaintenanceLoop {
		t.Fatalf("expected maintenance loop args, got %#v", container.Args)
	}
	if deployment.Spec.Strategy.Type != appsv1.RecreateDeploymentStrategyType {
		t.Fatalf("expected Recreate strategy in maintenance mode, got %s", deployment.Spec.Strategy.Type)
	}
	if container.LivenessProbe != nil {
		t.Fatalf("expected liveness probe to be disabled in maintenance mode, got %#v", container.LivenessProbe)
	}
	if container.ReadinessProbe == nil || container.ReadinessProbe.Exec == nil || len(container.ReadinessProbe.Exec.Command) != 3 || container.ReadinessProbe.Exec.Command[2] != connectionManagerMaintenanceReadinessCommand {
		t.Fatalf("expected failing maintenance readiness probe, got %#v", container.ReadinessProbe)
	}
	envs := map[string]string{}
	for _, env := range container.Env {
		envs[env.Name] = env.Value
	}
	if envs["CMAN_MAINTENANCE_MODE"] != "true" {
		t.Fatalf("expected maintenance environment marker, got %#v", envs)
	}
}

func TestBuildConnectionManagerServiceUsesRuntimePortAsTarget(t *testing.T) {
	inst := &networkv4.ConnectionManager{
		ObjectMeta: metav1.ObjectMeta{Name: "cman", Namespace: "net"},
		Spec: networkv4.ConnectionManagerSpec{
			Runtime: networkv4.ConnectionManagerRuntimeSpec{Image: "oracle/cman:23.8.0", Port: 1621},
			Service: networkv4.ConnectionManagerServiceSpec{
				Internal: networkv4.ConnectionManagerServiceEndpointSpec{Port: 1521},
			},
		},
	}
	service := buildConnectionManagerService(inst, "internal")
	if len(service.Spec.Ports) != 1 || service.Spec.Ports[0].Port != 1521 || service.Spec.Ports[0].TargetPort.IntValue() != 1621 {
		t.Fatalf("expected Service port 1521 to target runtime port 1621, got %#v", service.Spec.Ports)
	}
}
