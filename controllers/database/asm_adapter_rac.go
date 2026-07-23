package controllers

import (
	racdb "github.com/oracle/oracle-database-operator/apis/database/v4"
	sharedasm "github.com/oracle/oracle-database-operator/commons/crs/asm"
	raccommon "github.com/oracle/oracle-database-operator/commons/crs/rac"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type racAsmAdapter struct {
	obj *racdb.RacDatabase
	cl  client.Client
}

func newRacAsmAdapter(obj *racdb.RacDatabase, cl client.Client) *racAsmAdapter {
	return &racAsmAdapter{obj: obj, cl: cl}
}

func (a *racAsmAdapter) GetDiskGroups() []sharedasm.DiskGroup {
	out := make([]sharedasm.DiskGroup, 0, len(a.obj.Spec.AsmStorageDetails))
	for _, dg := range a.obj.Spec.AsmStorageDetails {
		out = append(out, sharedasm.DiskGroup{
			Name:               dg.Name,
			Type:               string(dg.Type),
			Redundancy:         dg.Redundancy,
			Disks:              dg.Disks,
			AutoUpdate:         dg.AutoUpdate,
			StorageClass:       dg.StorageClass,
			AccessMode:         dg.AccessMode,
			AsmStorageSizeInGb: dg.AsmStorageSizeInGb,
		})
	}
	return out
}

func (a *racAsmAdapter) SetDiskGroups(in []sharedasm.DiskGroup) {
	out := make([]racdb.AsmDiskGroupDetails, 0, len(in))
	for _, dg := range in {
		out = append(out, racdb.AsmDiskGroupDetails{
			Name:               dg.Name,
			Type:               racdb.AsmDiskDGTypes(dg.Type),
			Redundancy:         dg.Redundancy,
			Disks:              dg.Disks,
			AutoUpdate:         dg.AutoUpdate,
			StorageClass:       dg.StorageClass,
			AccessMode:         dg.AccessMode,
			AsmStorageSizeInGb: dg.AsmStorageSizeInGb,
		})
	}
	a.obj.Spec.AsmStorageDetails = out
}

func (a *racAsmAdapter) GetDbCharSet() string {
	if a.obj.Spec.ConfigParams == nil {
		return ""
	}
	return a.obj.Spec.ConfigParams.DbCharSet
}

func (a *racAsmAdapter) SetDbCharSet(v string) {
	if a.obj.Spec.ConfigParams == nil {
		a.obj.Spec.ConfigParams = &racdb.RacInitParams{}
	}
	a.obj.Spec.ConfigParams.DbCharSet = v
}

func (a *racAsmAdapter) LookupRspData(key, cmName, fileName string) (string, error) {
	return raccommon.CheckRspData(a.obj, a.cl, key, cmName, fileName)
}

func (a *racAsmAdapter) CrsType() string      { return string(racdb.CrsAsmDiskDg) }
func (a *racAsmAdapter) DataType() string     { return string(racdb.DbDataDiskDg) }
func (a *racAsmAdapter) RecoveryType() string { return string(racdb.DbRecoveryDiskDg) }

func (a *racAsmAdapter) CrsLookupKeys() []string {
	return []string{"oracle.install.asm.diskGroup.name", "diskGroupName", "datafileDestination", "db_create_file_dest"}
}

func (a *racAsmAdapter) DefaultCrsName() string    { return "+DATA" }
func (a *racAsmAdapter) DefaultRedundancy() string { return "EXTERNAL" }

func (a *racAsmAdapter) GetAsmGroups() []sharedasm.DiskGroup { return a.GetDiskGroups() }
