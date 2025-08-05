package v1alpha1

import (
	"errors"

	v4 "github.com/oracle/oracle-database-operator/apis/database/v4"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

// ConvertTo converts this AutonomousDatabase to the Hub version (v4).
func (src *AutonomousDatabase) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v4.AutonomousDatabase)
	// Convert the Spec
	dst.Spec.Action = src.Spec.Action

	// Details
	dst.Spec.Details.Id = src.Spec.Details.Id
	dst.Spec.Details.CompartmentId = src.Spec.Details.CompartmentId
	dst.Spec.Details.AutonomousContainerDatabase.K8sAcd.Name = src.Spec.Details.AutonomousContainerDatabase.K8sAcd.Name
	dst.Spec.Details.AutonomousContainerDatabase.OciAcd.Id = src.Spec.Details.AutonomousContainerDatabase.OciAcd.Id
	dst.Spec.Details.DisplayName = src.Spec.Details.DisplayName
	dst.Spec.Details.DbName = src.Spec.Details.DbName
	dst.Spec.Details.DbWorkload = src.Spec.Details.DbWorkload
	dst.Spec.Details.LicenseModel = src.Spec.Details.LicenseModel
	dst.Spec.Details.DbVersion = src.Spec.Details.DbVersion
	dst.Spec.Details.DataStorageSizeInTBs = src.Spec.Details.DataStorageSizeInTBs
	dst.Spec.Details.CpuCoreCount = src.Spec.Details.CpuCoreCount
	dst.Spec.Details.ComputeModel = src.Spec.Details.ComputeModel
	dst.Spec.Details.ComputeCount = src.Spec.Details.ComputeCount
	dst.Spec.Details.OcpuCount = src.Spec.Details.OcpuCount
	dst.Spec.Details.AdminPassword.K8sSecret.Name = src.Spec.Details.AdminPassword.K8sSecret.Name
	dst.Spec.Details.AdminPassword.OciSecret.Id = src.Spec.Details.AdminPassword.OciSecret.Id
	dst.Spec.Details.IsAutoScalingEnabled = src.Spec.Details.IsAutoScalingEnabled
	dst.Spec.Details.IsDedicated = src.Spec.Details.IsDedicated
	dst.Spec.Details.IsFreeTier = src.Spec.Details.IsFreeTier
	dst.Spec.Details.IsAccessControlEnabled = src.Spec.Details.IsAccessControlEnabled
	dst.Spec.Details.WhitelistedIps = src.Spec.Details.WhitelistedIps
	dst.Spec.Details.SubnetId = src.Spec.Details.SubnetId
	dst.Spec.Details.NsgIds = src.Spec.Details.NsgIds
	dst.Spec.Details.PrivateEndpointLabel = src.Spec.Details.PrivateEndpointLabel
	dst.Spec.Details.IsMtlsConnectionRequired = src.Spec.Details.IsMtlsConnectionRequired
	dst.Spec.Details.FreeformTags = src.Spec.Details.FreeformTags

	// Clone
	dst.Spec.Clone.CompartmentId = src.Spec.Clone.CompartmentId
	dst.Spec.Clone.AutonomousContainerDatabase.K8sAcd.Name = src.Spec.Clone.AutonomousContainerDatabase.K8sAcd.Name
	dst.Spec.Clone.AutonomousContainerDatabase.OciAcd.Id = src.Spec.Clone.AutonomousContainerDatabase.OciAcd.Id
	dst.Spec.Clone.DisplayName = src.Spec.Clone.DisplayName
	dst.Spec.Clone.DbName = src.Spec.Clone.DbName
	dst.Spec.Clone.DbWorkload = src.Spec.Clone.DbWorkload
	dst.Spec.Clone.LicenseModel = src.Spec.Clone.LicenseModel
	dst.Spec.Clone.DbVersion = src.Spec.Clone.DbVersion
	dst.Spec.Clone.DataStorageSizeInTBs = src.Spec.Clone.DataStorageSizeInTBs
	dst.Spec.Clone.CpuCoreCount = src.Spec.Clone.CpuCoreCount
	dst.Spec.Clone.ComputeModel = src.Spec.Clone.ComputeModel
	dst.Spec.Clone.ComputeCount = src.Spec.Clone.ComputeCount
	dst.Spec.Clone.OcpuCount = src.Spec.Clone.OcpuCount
	dst.Spec.Clone.AdminPassword.K8sSecret.Name = src.Spec.Clone.AdminPassword.K8sSecret.Name
	dst.Spec.Clone.AdminPassword.OciSecret.Id = src.Spec.Clone.AdminPassword.OciSecret.Id
	dst.Spec.Clone.IsAutoScalingEnabled = src.Spec.Clone.IsAutoScalingEnabled
	dst.Spec.Clone.IsDedicated = src.Spec.Clone.IsDedicated
	dst.Spec.Clone.IsFreeTier = src.Spec.Clone.IsFreeTier
	dst.Spec.Clone.IsAccessControlEnabled = src.Spec.Clone.IsAccessControlEnabled
	dst.Spec.Clone.WhitelistedIps = src.Spec.Clone.WhitelistedIps
	dst.Spec.Clone.SubnetId = src.Spec.Clone.SubnetId
	dst.Spec.Clone.NsgIds = src.Spec.Clone.NsgIds
	dst.Spec.Clone.PrivateEndpointLabel = src.Spec.Clone.PrivateEndpointLabel
	dst.Spec.Clone.IsMtlsConnectionRequired = src.Spec.Clone.IsMtlsConnectionRequired
	dst.Spec.Clone.FreeformTags = src.Spec.Clone.FreeformTags
	dst.Spec.Clone.CloneType = src.Spec.Clone.CloneType

	// Wallet
	dst.Spec.Wallet.Name = src.Spec.Wallet.Name
	dst.Spec.Wallet.Password.K8sSecret.Name = src.Spec.Wallet.Password.K8sSecret.Name
	dst.Spec.Wallet.Password.OciSecret.Id = src.Spec.Wallet.Password.OciSecret.Id

	dst.Spec.OciConfig.ConfigMapName = src.Spec.OciConfig.ConfigMapName
	dst.Spec.OciConfig.SecretName = src.Spec.OciConfig.SecretName

	dst.Spec.HardLink = src.Spec.HardLink

	// Convert the Status
	dst.Status.LifecycleState = src.Status.LifecycleState
	dst.Status.TimeCreated = src.Status.TimeCreated
	dst.Status.WalletExpiringDate = src.Status.WalletExpiringDate

	// convert status.allConnectionStrings
	if src.Status.AllConnectionStrings != nil {
		for _, srcProfile := range src.Status.AllConnectionStrings {
			dstProfile := v4.ConnectionStringProfile{}

			// convert status.allConnectionStrings[i].tlsAuthentication
			if val, ok := v4.GetTLSAuthenticationEnumFromString(string(srcProfile.TLSAuthentication)); !ok {
				return errors.New("Unable to convert to TLSAuthenticationEnum: " + string(srcProfile.TLSAuthentication))
			} else {
				dstProfile.TLSAuthentication = val
			}

			// convert status.allConnectionStrings[i].connectionStrings
			dstProfile.ConnectionStrings = make([]v4.ConnectionStringSpec, len(srcProfile.ConnectionStrings))
			for i, v := range srcProfile.ConnectionStrings {
				dstProfile.ConnectionStrings[i].TNSName = v.TNSName
				dstProfile.ConnectionStrings[i].ConnectionString = v.ConnectionString
			}

			dst.Status.AllConnectionStrings = append(dst.Status.AllConnectionStrings, dstProfile)
		}
	}

	dst.Status.Conditions = src.Status.Conditions

	dst.ObjectMeta = src.ObjectMeta
	return nil
}

// ConvertFrom converts from the Hub version (v4) to v1alpha1
func (dst *AutonomousDatabase) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v4.AutonomousDatabase)

	// Convert the Spec
	dst.Spec.Action = src.Spec.Action

	// Details
	dst.Spec.Details.Id = src.Spec.Details.Id
	dst.Spec.Details.CompartmentId = src.Spec.Details.CompartmentId
	dst.Spec.Details.AutonomousContainerDatabase.K8sAcd.Name = src.Spec.Details.AutonomousContainerDatabase.K8sAcd.Name
	dst.Spec.Details.AutonomousContainerDatabase.OciAcd.Id = src.Spec.Details.AutonomousContainerDatabase.OciAcd.Id
	dst.Spec.Details.DisplayName = src.Spec.Details.DisplayName
	dst.Spec.Details.DbName = src.Spec.Details.DbName
	dst.Spec.Details.DbWorkload = src.Spec.Details.DbWorkload
	dst.Spec.Details.LicenseModel = src.Spec.Details.LicenseModel
	dst.Spec.Details.DbVersion = src.Spec.Details.DbVersion
	dst.Spec.Details.DataStorageSizeInTBs = src.Spec.Details.DataStorageSizeInTBs
	dst.Spec.Details.CpuCoreCount = src.Spec.Details.CpuCoreCount
	dst.Spec.Details.ComputeModel = src.Spec.Details.ComputeModel
	dst.Spec.Details.ComputeCount = src.Spec.Details.ComputeCount
	dst.Spec.Details.OcpuCount = src.Spec.Details.OcpuCount
	dst.Spec.Details.AdminPassword.K8sSecret.Name = src.Spec.Details.AdminPassword.K8sSecret.Name
	dst.Spec.Details.AdminPassword.OciSecret.Id = src.Spec.Details.AdminPassword.OciSecret.Id
	dst.Spec.Details.IsAutoScalingEnabled = src.Spec.Details.IsAutoScalingEnabled
	dst.Spec.Details.IsDedicated = src.Spec.Details.IsDedicated
	dst.Spec.Details.IsFreeTier = src.Spec.Details.IsFreeTier
	dst.Spec.Details.IsAccessControlEnabled = src.Spec.Details.IsAccessControlEnabled
	dst.Spec.Details.WhitelistedIps = src.Spec.Details.WhitelistedIps
	dst.Spec.Details.SubnetId = src.Spec.Details.SubnetId
	dst.Spec.Details.NsgIds = src.Spec.Details.NsgIds
	dst.Spec.Details.PrivateEndpointLabel = src.Spec.Details.PrivateEndpointLabel
	dst.Spec.Details.IsMtlsConnectionRequired = src.Spec.Details.IsMtlsConnectionRequired
	dst.Spec.Details.FreeformTags = src.Spec.Details.FreeformTags

	// Clone
	dst.Spec.Clone.CompartmentId = src.Spec.Clone.CompartmentId
	dst.Spec.Clone.AutonomousContainerDatabase.K8sAcd.Name = src.Spec.Clone.AutonomousContainerDatabase.K8sAcd.Name
	dst.Spec.Clone.AutonomousContainerDatabase.OciAcd.Id = src.Spec.Clone.AutonomousContainerDatabase.OciAcd.Id
	dst.Spec.Clone.DisplayName = src.Spec.Clone.DisplayName
	dst.Spec.Clone.DbName = src.Spec.Clone.DbName
	dst.Spec.Clone.DbWorkload = src.Spec.Clone.DbWorkload
	dst.Spec.Clone.LicenseModel = src.Spec.Clone.LicenseModel
	dst.Spec.Clone.DbVersion = src.Spec.Clone.DbVersion
	dst.Spec.Clone.DataStorageSizeInTBs = src.Spec.Clone.DataStorageSizeInTBs
	dst.Spec.Clone.CpuCoreCount = src.Spec.Clone.CpuCoreCount
	dst.Spec.Clone.ComputeModel = src.Spec.Clone.ComputeModel
	dst.Spec.Clone.ComputeCount = src.Spec.Clone.ComputeCount
	dst.Spec.Clone.OcpuCount = src.Spec.Clone.OcpuCount
	dst.Spec.Clone.AdminPassword.K8sSecret.Name = src.Spec.Clone.AdminPassword.K8sSecret.Name
	dst.Spec.Clone.AdminPassword.OciSecret.Id = src.Spec.Clone.AdminPassword.OciSecret.Id
	dst.Spec.Clone.IsAutoScalingEnabled = src.Spec.Clone.IsAutoScalingEnabled
	dst.Spec.Clone.IsDedicated = src.Spec.Clone.IsDedicated
	dst.Spec.Clone.IsFreeTier = src.Spec.Clone.IsFreeTier
	dst.Spec.Clone.IsAccessControlEnabled = src.Spec.Clone.IsAccessControlEnabled
	dst.Spec.Clone.WhitelistedIps = src.Spec.Clone.WhitelistedIps
	dst.Spec.Clone.SubnetId = src.Spec.Clone.SubnetId
	dst.Spec.Clone.NsgIds = src.Spec.Clone.NsgIds
	dst.Spec.Clone.PrivateEndpointLabel = src.Spec.Clone.PrivateEndpointLabel
	dst.Spec.Clone.IsMtlsConnectionRequired = src.Spec.Clone.IsMtlsConnectionRequired
	dst.Spec.Clone.FreeformTags = src.Spec.Clone.FreeformTags
	dst.Spec.Clone.CloneType = src.Spec.Clone.CloneType

	// Wallet
	dst.Spec.Wallet.Name = src.Spec.Wallet.Name
	dst.Spec.Wallet.Password.K8sSecret.Name = src.Spec.Wallet.Password.K8sSecret.Name
	dst.Spec.Wallet.Password.OciSecret.Id = src.Spec.Wallet.Password.OciSecret.Id

	dst.Spec.OciConfig.ConfigMapName = src.Spec.OciConfig.ConfigMapName
	dst.Spec.OciConfig.SecretName = src.Spec.OciConfig.SecretName

	dst.Spec.HardLink = src.Spec.HardLink

	// Convert the Status
	dst.Status.LifecycleState = src.Status.LifecycleState
	dst.Status.TimeCreated = src.Status.TimeCreated
	dst.Status.WalletExpiringDate = src.Status.WalletExpiringDate

	// convert status.allConnectionStrings
	if src.Status.AllConnectionStrings != nil {
		for _, srcProfile := range src.Status.AllConnectionStrings {
			dstProfile := ConnectionStringProfile{}

			// convert status.allConnectionStrings[i].tlsAuthentication
			if val, ok := GetTLSAuthenticationEnumFromString(string(srcProfile.TLSAuthentication)); !ok {
				return errors.New("Unable to convert to TLSAuthenticationEnum: " + string(srcProfile.TLSAuthentication))
			} else {
				dstProfile.TLSAuthentication = val
			}

			// convert status.allConnectionStrings[i].connectionStrings
			dstProfile.ConnectionStrings = make([]ConnectionStringSpec, len(srcProfile.ConnectionStrings))
			for i, v := range srcProfile.ConnectionStrings {
				dstProfile.ConnectionStrings[i].TNSName = v.TNSName
				dstProfile.ConnectionStrings[i].ConnectionString = v.ConnectionString
			}

			dst.Status.AllConnectionStrings = append(dst.Status.AllConnectionStrings, dstProfile)
		}
	}

	dst.Status.Conditions = src.Status.Conditions

	dst.ObjectMeta = src.ObjectMeta
	return nil
}

func (src *AutonomousDatabaseBackup) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v4.AutonomousDatabaseBackup)

	dst.Spec.Target.K8sAdb.Name = src.Spec.Target.K8sAdb.Name
	dst.Spec.Target.OciAdb.Ocid = src.Spec.Target.OciAdb.Ocid
	dst.Spec.DisplayName = src.Spec.DisplayName
	dst.Spec.AutonomousDatabaseBackupOCID = src.Spec.AutonomousDatabaseBackupOCID
	dst.Spec.IsLongTermBackup = src.Spec.IsLongTermBackup
	dst.Spec.RetentionPeriodInDays = src.Spec.RetentionPeriodInDays
	dst.Spec.OCIConfig.ConfigMapName = src.Spec.OCIConfig.ConfigMapName
	dst.Spec.OCIConfig.SecretName = src.Spec.OCIConfig.SecretName

	dst.Status.LifecycleState = src.Status.LifecycleState
	dst.Status.Type = src.Status.Type
	dst.Status.IsAutomatic = src.Status.IsAutomatic
	dst.Status.TimeStarted = src.Status.TimeStarted
	dst.Status.TimeEnded = src.Status.TimeEnded
	dst.Status.AutonomousDatabaseOCID = src.Status.AutonomousDatabaseOCID
	dst.Status.CompartmentOCID = src.Status.CompartmentOCID
	dst.Status.DBName = src.Status.DBName
	dst.Status.DBDisplayName = src.Status.DBDisplayName

	dst.ObjectMeta = src.ObjectMeta
	return nil
}

func (dst *AutonomousDatabaseBackup) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v4.AutonomousDatabaseBackup)

	dst.Spec.Target.K8sAdb.Name = src.Spec.Target.K8sAdb.Name
	dst.Spec.Target.OciAdb.Ocid = src.Spec.Target.OciAdb.Ocid
	dst.Spec.DisplayName = src.Spec.DisplayName
	dst.Spec.AutonomousDatabaseBackupOCID = src.Spec.AutonomousDatabaseBackupOCID
	dst.Spec.IsLongTermBackup = src.Spec.IsLongTermBackup
	dst.Spec.RetentionPeriodInDays = src.Spec.RetentionPeriodInDays
	dst.Spec.OCIConfig.ConfigMapName = src.Spec.OCIConfig.ConfigMapName
	dst.Spec.OCIConfig.SecretName = src.Spec.OCIConfig.SecretName

	dst.Status.LifecycleState = src.Status.LifecycleState
	dst.Status.Type = src.Status.Type
	dst.Status.IsAutomatic = src.Status.IsAutomatic
	dst.Status.TimeStarted = src.Status.TimeStarted
	dst.Status.TimeEnded = src.Status.TimeEnded
	dst.Status.AutonomousDatabaseOCID = src.Status.AutonomousDatabaseOCID
	dst.Status.CompartmentOCID = src.Status.CompartmentOCID
	dst.Status.DBName = src.Status.DBName
	dst.Status.DBDisplayName = src.Status.DBDisplayName

	dst.ObjectMeta = src.ObjectMeta
	return nil
}

func (src *AutonomousDatabaseRestore) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v4.AutonomousDatabaseRestore)

	dst.Spec.Target.K8sAdb.Name = src.Spec.Target.K8sAdb.Name
	dst.Spec.Target.OciAdb.Ocid = src.Spec.Target.OciAdb.Ocid
	dst.Spec.Source.K8sAdbBackup.Name = src.Spec.Source.K8sAdbBackup.Name
	dst.Spec.Source.PointInTime.Timestamp = src.Spec.Source.PointInTime.Timestamp
	dst.Spec.OCIConfig.ConfigMapName = src.Spec.OCIConfig.ConfigMapName
	dst.Spec.OCIConfig.SecretName = src.Spec.OCIConfig.SecretName

	dst.Status.DisplayName = src.Status.DisplayName
	dst.Status.TimeAccepted = src.Status.TimeAccepted
	dst.Status.TimeStarted = src.Status.TimeStarted
	dst.Status.TimeEnded = src.Status.TimeEnded
	dst.Status.DbName = src.Status.DbName
	dst.Status.WorkRequestOCID = src.Status.WorkRequestOCID
	dst.Status.Status = src.Status.Status

	dst.ObjectMeta = src.ObjectMeta
	return nil
}

func (dst *AutonomousDatabaseRestore) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v4.AutonomousDatabaseRestore)

	dst.Spec.Target.K8sAdb.Name = src.Spec.Target.K8sAdb.Name
	dst.Spec.Target.OciAdb.Ocid = src.Spec.Target.OciAdb.Ocid
	dst.Spec.Source.K8sAdbBackup.Name = src.Spec.Source.K8sAdbBackup.Name
	dst.Spec.Source.PointInTime.Timestamp = src.Spec.Source.PointInTime.Timestamp
	dst.Spec.OCIConfig.ConfigMapName = src.Spec.OCIConfig.ConfigMapName
	dst.Spec.OCIConfig.SecretName = src.Spec.OCIConfig.SecretName

	dst.Status.DisplayName = src.Status.DisplayName
	dst.Status.TimeAccepted = src.Status.TimeAccepted
	dst.Status.TimeStarted = src.Status.TimeStarted
	dst.Status.TimeEnded = src.Status.TimeEnded
	dst.Status.DbName = src.Status.DbName
	dst.Status.WorkRequestOCID = src.Status.WorkRequestOCID
	dst.Status.Status = src.Status.Status

	dst.ObjectMeta = src.ObjectMeta
	return nil
}

func (src *AutonomousContainerDatabase) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v4.AutonomousContainerDatabase)

	dst.Spec.AutonomousContainerDatabaseOCID = src.Spec.AutonomousContainerDatabaseOCID
	dst.Spec.CompartmentOCID = src.Spec.CompartmentOCID
	dst.Spec.DisplayName = src.Spec.DisplayName
	dst.Spec.AutonomousExadataVMClusterOCID = src.Spec.AutonomousExadataVMClusterOCID
	dst.Spec.PatchModel = src.Spec.PatchModel

	if val, ok := v4.GetAcdActionEnumFromString(string(src.Spec.Action)); !ok {
		return errors.New("Unable to convert to AcdActionEnum: " + string(src.Spec.Action))
	} else {
		dst.Spec.Action = val
	}

	dst.Spec.FreeformTags = src.Spec.FreeformTags
	dst.Spec.OCIConfig.ConfigMapName = src.Spec.OCIConfig.ConfigMapName
	dst.Spec.OCIConfig.SecretName = src.Spec.OCIConfig.SecretName
	dst.Spec.HardLink = src.Spec.HardLink

	dst.Status.LifecycleState = src.Status.LifecycleState
	dst.Status.TimeCreated = src.Status.TimeCreated

	dst.ObjectMeta = src.ObjectMeta
	return nil
}

func (dst *AutonomousContainerDatabase) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v4.AutonomousContainerDatabase)

	dst.Spec.AutonomousContainerDatabaseOCID = src.Spec.AutonomousContainerDatabaseOCID
	dst.Spec.CompartmentOCID = src.Spec.CompartmentOCID
	dst.Spec.DisplayName = src.Spec.DisplayName
	dst.Spec.AutonomousExadataVMClusterOCID = src.Spec.AutonomousExadataVMClusterOCID
	dst.Spec.PatchModel = src.Spec.PatchModel

	if val, ok := GetAcdActionEnumFromString(string(src.Spec.Action)); !ok {
		return errors.New("Unable to convert to AcdActionEnum: " + string(src.Spec.Action))
	} else {
		dst.Spec.Action = val
	}

	dst.Spec.FreeformTags = src.Spec.FreeformTags
	dst.Spec.OCIConfig.ConfigMapName = src.Spec.OCIConfig.ConfigMapName
	dst.Spec.OCIConfig.SecretName = src.Spec.OCIConfig.SecretName
	dst.Spec.HardLink = src.Spec.HardLink

	dst.Status.LifecycleState = src.Status.LifecycleState
	dst.Status.TimeCreated = src.Status.TimeCreated

	dst.ObjectMeta = src.ObjectMeta
	return nil
}
