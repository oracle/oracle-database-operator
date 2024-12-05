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
	dst.Spec.Details.AutonomousDatabaseOCID = src.Spec.Details.AutonomousDatabaseOCID
	dst.Spec.Details.CompartmentOCID = src.Spec.Details.CompartmentOCID
	dst.Spec.Details.AutonomousContainerDatabase.K8sACD.Name = src.Spec.Details.AutonomousContainerDatabase.K8sACD.Name
	dst.Spec.Details.AutonomousContainerDatabase.OCIACD.OCID = src.Spec.Details.AutonomousContainerDatabase.OCIACD.OCID
	dst.Spec.Details.DisplayName = src.Spec.Details.DisplayName
	dst.Spec.Details.DbName = src.Spec.Details.DbName
	dst.Spec.Details.DbWorkload = src.Spec.Details.DbWorkload
	dst.Spec.Details.LicenseModel = src.Spec.Details.LicenseModel
	dst.Spec.Details.DbVersion = src.Spec.Details.DbVersion
	dst.Spec.Details.DataStorageSizeInTBs = src.Spec.Details.DataStorageSizeInTBs
	dst.Spec.Details.CPUCoreCount = src.Spec.Details.CPUCoreCount
	dst.Spec.Details.AdminPassword.K8sSecret.Name = src.Spec.Details.AdminPassword.K8sSecret.Name
	dst.Spec.Details.AdminPassword.OCISecret.OCID = src.Spec.Details.AdminPassword.OCISecret.OCID
	dst.Spec.Details.IsAutoScalingEnabled = src.Spec.Details.IsAutoScalingEnabled
	dst.Spec.Details.IsDedicated = src.Spec.Details.IsDedicated
	dst.Spec.Details.LifecycleState = src.Spec.Details.LifecycleState

	if val, ok := v4.GetNetworkAccessTypeFromString(string(src.Spec.Details.NetworkAccess.AccessType)); !ok {
		return errors.New("Unable to convert to NetworkAccessTypeEnum: " + string(src.Spec.Details.NetworkAccess.AccessType))
	} else {
		dst.Spec.Details.NetworkAccess.AccessType = val
	}
	dst.Spec.Details.NetworkAccess.IsAccessControlEnabled = src.Spec.Details.NetworkAccess.IsAccessControlEnabled
	dst.Spec.Details.NetworkAccess.AccessControlList = src.Spec.Details.NetworkAccess.AccessControlList
	dst.Spec.Details.NetworkAccess.PrivateEndpoint.SubnetOCID = src.Spec.Details.NetworkAccess.PrivateEndpoint.SubnetOCID
	dst.Spec.Details.NetworkAccess.PrivateEndpoint.NsgOCIDs = src.Spec.Details.NetworkAccess.PrivateEndpoint.NsgOCIDs
	dst.Spec.Details.NetworkAccess.PrivateEndpoint.HostnamePrefix = src.Spec.Details.NetworkAccess.PrivateEndpoint.HostnamePrefix
	dst.Spec.Details.NetworkAccess.IsMTLSConnectionRequired = src.Spec.Details.NetworkAccess.IsMTLSConnectionRequired

	dst.Spec.Details.FreeformTags = src.Spec.Details.FreeformTags
	dst.Spec.Details.Wallet.Name = src.Spec.Details.Wallet.Name
	dst.Spec.Details.Wallet.Password.K8sSecret.Name = src.Spec.Details.Wallet.Password.K8sSecret.Name
	dst.Spec.Details.Wallet.Password.OCISecret.OCID = src.Spec.Details.Wallet.Password.OCISecret.OCID

	dst.Spec.OCIConfig.ConfigMapName = src.Spec.OCIConfig.ConfigMapName
	dst.Spec.OCIConfig.SecretName = src.Spec.OCIConfig.SecretName

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
	dst.Spec.Details.AutonomousDatabaseOCID = src.Spec.Details.AutonomousDatabaseOCID
	dst.Spec.Details.CompartmentOCID = src.Spec.Details.CompartmentOCID
	dst.Spec.Details.AutonomousContainerDatabase.K8sACD.Name = src.Spec.Details.AutonomousContainerDatabase.K8sACD.Name
	dst.Spec.Details.AutonomousContainerDatabase.OCIACD.OCID = src.Spec.Details.AutonomousContainerDatabase.OCIACD.OCID
	dst.Spec.Details.DisplayName = src.Spec.Details.DisplayName
	dst.Spec.Details.DbName = src.Spec.Details.DbName
	dst.Spec.Details.DbWorkload = src.Spec.Details.DbWorkload
	dst.Spec.Details.LicenseModel = src.Spec.Details.LicenseModel
	dst.Spec.Details.DbVersion = src.Spec.Details.DbVersion
	dst.Spec.Details.DataStorageSizeInTBs = src.Spec.Details.DataStorageSizeInTBs
	dst.Spec.Details.CPUCoreCount = src.Spec.Details.CPUCoreCount
	dst.Spec.Details.AdminPassword.K8sSecret.Name = src.Spec.Details.AdminPassword.K8sSecret.Name
	dst.Spec.Details.AdminPassword.OCISecret.OCID = src.Spec.Details.AdminPassword.OCISecret.OCID
	dst.Spec.Details.IsAutoScalingEnabled = src.Spec.Details.IsAutoScalingEnabled
	dst.Spec.Details.IsDedicated = src.Spec.Details.IsDedicated
	dst.Spec.Details.LifecycleState = src.Spec.Details.LifecycleState

	if val, ok := GetNetworkAccessTypeFromString(string(src.Spec.Details.NetworkAccess.AccessType)); !ok {
		return errors.New("Unable to convert to NetworkAccessTypeEnum: " + string(src.Spec.Details.NetworkAccess.AccessType))
	} else {
		dst.Spec.Details.NetworkAccess.AccessType = val
	}
	dst.Spec.Details.NetworkAccess.IsAccessControlEnabled = src.Spec.Details.NetworkAccess.IsAccessControlEnabled
	dst.Spec.Details.NetworkAccess.AccessControlList = src.Spec.Details.NetworkAccess.AccessControlList
	dst.Spec.Details.NetworkAccess.PrivateEndpoint.SubnetOCID = src.Spec.Details.NetworkAccess.PrivateEndpoint.SubnetOCID
	dst.Spec.Details.NetworkAccess.PrivateEndpoint.NsgOCIDs = src.Spec.Details.NetworkAccess.PrivateEndpoint.NsgOCIDs
	dst.Spec.Details.NetworkAccess.PrivateEndpoint.HostnamePrefix = src.Spec.Details.NetworkAccess.PrivateEndpoint.HostnamePrefix
	dst.Spec.Details.NetworkAccess.IsMTLSConnectionRequired = src.Spec.Details.NetworkAccess.IsMTLSConnectionRequired

	dst.Spec.Details.FreeformTags = src.Spec.Details.FreeformTags
	dst.Spec.Details.Wallet.Name = src.Spec.Details.Wallet.Name
	dst.Spec.Details.Wallet.Password.K8sSecret.Name = src.Spec.Details.Wallet.Password.K8sSecret.Name
	dst.Spec.Details.Wallet.Password.OCISecret.OCID = src.Spec.Details.Wallet.Password.OCISecret.OCID

	dst.Spec.OCIConfig.ConfigMapName = src.Spec.OCIConfig.ConfigMapName
	dst.Spec.OCIConfig.SecretName = src.Spec.OCIConfig.SecretName

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

	dst.Spec.Target.K8sADB.Name = src.Spec.Target.K8sADB.Name
	dst.Spec.Target.OCIADB.OCID = src.Spec.Target.OCIADB.OCID
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

	dst.Spec.Target.K8sADB.Name = src.Spec.Target.K8sADB.Name
	dst.Spec.Target.OCIADB.OCID = src.Spec.Target.OCIADB.OCID
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

	dst.Spec.Target.K8sADB.Name = src.Spec.Target.K8sADB.Name
	dst.Spec.Target.OCIADB.OCID = src.Spec.Target.OCIADB.OCID
	dst.Spec.Source.K8sADBBackup.Name = src.Spec.Source.K8sADBBackup.Name
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

	dst.Spec.Target.K8sADB.Name = src.Spec.Target.K8sADB.Name
	dst.Spec.Target.OCIADB.OCID = src.Spec.Target.OCIADB.OCID
	dst.Spec.Source.K8sADBBackup.Name = src.Spec.Source.K8sADBBackup.Name
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
