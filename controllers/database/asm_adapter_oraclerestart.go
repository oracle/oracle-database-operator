package controllers

import (
	oraclerestartdb "github.com/oracle/oracle-database-operator/apis/database/v4"
	sharedasm "github.com/oracle/oracle-database-operator/commons/crs/asm"
	oraclerestartcommon "github.com/oracle/oracle-database-operator/commons/crs/restart"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type oracleRestartAsmAdapter struct {
	obj *oraclerestartdb.OracleRestart
	cl  client.Client
}

func newOracleRestartAsmAdapter(obj *oraclerestartdb.OracleRestart, cl client.Client) *oracleRestartAsmAdapter {
	return &oracleRestartAsmAdapter{obj: obj, cl: cl}
}

func (a *oracleRestartAsmAdapter) GetDiskGroups() []sharedasm.DiskGroup {
	out := make([]sharedasm.DiskGroup, 0, len(a.obj.Spec.AsmStorageDetails))
	for _, dg := range a.obj.Spec.AsmStorageDetails {
		out = append(out, sharedasm.DiskGroup{
			Name:               dg.Name,
			Type:               string(dg.Type),
			Redundancy:         dg.Redundancy,
			Disks:              dg.Disks,
			AutoUpdate:         dg.AutoUpdate,
			StorageClass:       dg.StorageClass,
			AsmStorageSizeInGb: dg.AsmStorageSizeInGb,
		})
	}
	return out
}

func (a *oracleRestartAsmAdapter) SetDiskGroups(in []sharedasm.DiskGroup) {
	out := make([]oraclerestartdb.AsmDiskGroupDetails, 0, len(in))
	for _, dg := range in {
		out = append(out, oraclerestartdb.AsmDiskGroupDetails{
			Name:               dg.Name,
			Type:               oraclerestartdb.AsmDiskDGTypes(dg.Type),
			Redundancy:         dg.Redundancy,
			Disks:              dg.Disks,
			AutoUpdate:         dg.AutoUpdate,
			StorageClass:       dg.StorageClass,
			AsmStorageSizeInGb: dg.AsmStorageSizeInGb,
		})
	}
	a.obj.Spec.AsmStorageDetails = out
}

func (a *oracleRestartAsmAdapter) GetDbCharSet() string {
	if a.obj.Spec.ConfigParams == nil {
		return ""
	}
	return a.obj.Spec.ConfigParams.DbCharSet
}

func (a *oracleRestartAsmAdapter) SetDbCharSet(v string) {
	if a.obj.Spec.ConfigParams == nil {
		a.obj.Spec.ConfigParams = &oraclerestartdb.InitParams{}
	}
	a.obj.Spec.ConfigParams.DbCharSet = v
}

func (a *oracleRestartAsmAdapter) LookupRspData(key, cmName, fileName string) (string, error) {
	return oraclerestartcommon.CheckRspData(a.obj, a.cl, key, cmName, fileName)
}

func (a *oracleRestartAsmAdapter) CrsType() string  { return string(oraclerestartdb.CrsAsmDiskDg) }
func (a *oracleRestartAsmAdapter) DataType() string { return string(oraclerestartdb.DbDataDiskDg) }
func (a *oracleRestartAsmAdapter) RecoveryType() string {
	return string(oraclerestartdb.DbRecoveryDiskDg)
}

func (a *oracleRestartAsmAdapter) CrsLookupKeys() []string {
	return []string{"oracle.install.asm.diskGroup.name", "diskGroupName"}
}

func (a *oracleRestartAsmAdapter) DefaultCrsName() string    { return "+DATA" }
func (a *oracleRestartAsmAdapter) DefaultRedundancy() string { return "EXTERNAL" }

func (a *oracleRestartAsmAdapter) GetAsmGroups() []sharedasm.DiskGroup { return a.GetDiskGroups() }
