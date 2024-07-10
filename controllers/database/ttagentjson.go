// Copyright (c) 2019-2021, Oracle and/or its affiliates. All rights reserved.
//
// Communication with the TimesTen Agent

package controllers

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strconv"

	timestenv2 "github.com/oracle/oracle-database-operator/apis/database/v1alpha1"
)

// Fetch an integer value from a JSON payload
// var x map[string]interface{}
// err := json.Unmarshal(yourJson, &x)
// ...
// var y int
// y, err := GetSimpleJsonInt64(&x, "foo")
func GetSimpleJsonInt64(j *map[string]interface{}, key string) (int64, error) {
	var y int64
	err := GetSimpleJsonValue(j, key, &y)
	if err != nil {
		return -1, err
	} else {
		return y, nil
	}
}

// Fetch an integer value from a JSON payload
// var x map[string]interface{}
// err := json.Unmarshal(yourJson, &x)
// ...
// var y int
// y, err := GetSimpleJsonInt(&x, "foo")
func GetSimpleJsonInt(j *map[string]interface{}, key string) (int, error) {
	var y int
	err := GetSimpleJsonValue(j, key, &y)
	if err != nil {
		return -1, err
	} else {
		return y, nil
	}
}

// Fetch a bool value from a JSON payload
// var x map[string]interface{}
// err := json.Unmarshal(yourJson, &x)
// ...
// var y bool
// y, err := GetSimpleJsonBool(&x, "foo")
func GetSimpleJsonBool(j *map[string]interface{}, key string) (bool, error) {
	var y bool
	err := GetSimpleJsonValue(j, key, &y)
	if err != nil {
		fmt.Println("GetSimpleJsonBool: " + err.Error())
		return false, err
	} else {
		return y, nil
	}
}

// Fetch a float value from a JSON payload
// var x map[string]interface{}
// err := json.Unmarshal(yourJson, &x)
// ...
// var y bool
// y, err := GetSimpleJsonBool(&x, "foo")
func GetSimpleJsonFloat32(j *map[string]interface{}, key string) (float32, error) {
	var y float32
	err := GetSimpleJsonValue(j, key, &y)
	if err != nil {
		fmt.Println("GetSimpleJsonFloat32: " + err.Error())
		return 0.0, err
	} else {
		return y, nil
	}
}

// Fetch a string value from a JSON payload
// var x map[string]interface{}
// err := json.Unmarshal(yourJson, &x)
// ...
// var y string
// y, err := GetSimpleJsonString(&x, "foo")
func GetSimpleJsonString(j *map[string]interface{}, key string) (string, error) {
	var y string
	err := GetSimpleJsonValue(j, key, &y)
	if err != nil {
		return "", err
	} else {
		return y, nil
	}
}

// Fetch a value from a JSON payload
// var x map[string]interface{}
// err := json.Unmarshal(yourJson, &x)
// ...
// var y string (or int or ...)
// err = GetSimpleJsonValue(&x, "foo", &y)
//
// If y is a string or int or other numeric type it gets
// the value of the {"foo": whatever} entry in the Json.
// If "foo" didn't exist in the json then y is unchanged.
//
// If y is a pointer to a string or int or other numeric
// type then y is either a pointer to a new value or it's nil
// (meaning that "foo" didn't exist)
// If there's a type conversion problem then err is non-nil
func GetSimpleJsonValue(j *map[string]interface{}, key string, out interface{}) error {

	// See what the return type is

	outv := reflect.ValueOf(out)
	outt := outv.Type()

	//fmt.Println("GetSimpleJsonValue called to find '" + key + "'")

	//if outv.CanSet() {
	//	fmt.Println("out can be set")
	//} else {
	//	fmt.Println("out can not be set")
	//}

	outk := outt.Kind()
	if outk != reflect.Ptr {
		return errors.New("GetSimpleJsonValue: out value not a pointer")
	}

	ooutv := outv.Elem()
	ooutk := ooutv.Kind()

	// See if the requested item exists in the json and is a supported type

	v, ok := (*j)[key]
	if !ok {
		return errors.New("GetSimpleJsonValue: '" + key + "' not found")
	}

	vv := reflect.ValueOf(v)
	vk := vv.Type().Kind()
	switch vk {
	case reflect.String:
		//fmt.Println("value is a string")
		switch ooutk {
		case reflect.String:
			//fmt.Println("out points to a string")
			ooutv.Set(vv)

		case reflect.Int, reflect.Int64:
			//fmt.Println("out points to an int")
			i, err := strconv.ParseInt(vv.String(), 10, 64)
			if err == nil {
				ooutv.SetInt(i)
			} else {
				return err
			}

		case reflect.Float64:
			//fmt.Println("out points to a float64")
			f, err := strconv.ParseFloat(vv.String(), 64)
			if err == nil {
				ooutv.SetFloat(f)
			} else {
				return err
			}

		case reflect.Float32:
			//fmt.Println("out points to a float32")
			f, err := strconv.ParseFloat(vv.String(), 64)
			if err == nil {
				ooutv.SetFloat(f)
			} else {
				return err
			}
		}

	case reflect.Bool:
		switch ooutk {
		case reflect.Bool:
			ooutv.Set(vv)
		default:
			// Handle other cases later...
		}

	case reflect.Float64:
		//fmt.Println("value is a float64")
		switch ooutk {
		case reflect.String:
			//fmt.Println("out points to a string")
			ooutv.SetString(fmt.Sprintf("%v", vv.Float()))

		case reflect.Int, reflect.Int64:
			//fmt.Println("out points to an int")
			ooutv.SetInt(int64(vv.Float()))

		case reflect.Float64:
			//fmt.Println("out points to a float64")
			ooutv.SetFloat(vv.Float())

		case reflect.Float32:
			//fmt.Println("out points to a float32")
			ooutv.SetFloat(vv.Float())
		}

	case reflect.Float32:
		//fmt.Println("value is a float32")
		switch ooutk {
		case reflect.String:
			//fmt.Println("out points to a string")
			ooutv.SetString(fmt.Sprintf("%v", vv.Float()))

		case reflect.Int, reflect.Int64:
			//fmt.Println("out points to an int")
			ooutv.SetInt(int64(vv.Float()))

		case reflect.Float64:
			//fmt.Println("out points to a float64")
			ooutv.SetFloat(vv.Float())

		case reflect.Float32:
			//fmt.Println("out points to a float32")
			ooutv.SetFloat(vv.Float())
		}

	case reflect.Slice:
		switch ooutk {
		case reflect.Slice:
			ooutv.Set(vv)
		default:
			return errors.New("GetSimpleJsonValue: '" + key + "' (slice) returned to unknown type")
		}

	default:
		return errors.New("GetSimpleJsonValue: '" + key + "' not a supported type")
	}

	return nil
}

// Redact passwords from a string we might log
func Redact(s string) string {
	s = regexp.MustCompile("-uid .+? ").ReplaceAllString(s, "-uid REDACTED ")
	s = regexp.MustCompile("-pwd .+? ").ReplaceAllString(s, "-pwd REDACTED ")
	s = regexp.MustCompile("-cacheUid .+? ").ReplaceAllString(s, "-cacheUid REDACTED ")
	s = regexp.MustCompile("-cachePwd .+? ").ReplaceAllString(s, "-cachdPwd REDACTED ")
	s = regexp.MustCompile("UID=.+?;").ReplaceAllString(s, "UID=REDACTED;")
	s = regexp.MustCompile("PWD=.+?;").ReplaceAllString(s, "PWD=REDACTED;")
	s = regexp.MustCompile("OraclePWD=.+?;").ReplaceAllString(s, "OraclePWD=REDACTED;")
	s = regexp.MustCompile("create user .+;").ReplaceAllString(s, "create user REDACTED;")
	s = regexp.MustCompile("sqlplus [[:alnum:]]+/[[:alnum:]]+@[[:alnum:]]+").ReplaceAllString(s, "sqlplus REDACTED;")
	return s
}

// Output from a GET request to the Agent

type TTAgentXlaSubscription struct {
	Bookmark string `json:"bookmark"`
	TblName  string `json:"tblname"`
	TblOwner string `json:"tblowner"`
}

type TTAgentXla struct {
	Id           string  `json:"id"`
	ReadLsnHigh  int64   `json:"readlsnhigh"`
	ReadLsnLow   int64   `json:"readlsnlow"`
	PurgeLsnHigh int64   `json:"purgelsnhigh"`
	PurgeLsnLow  int64   `json:"purgelsnlow"`
	Pid          int64   `json:"pid"`
	InUse        string  `json:"inuse"`
	Replicated   *string `json:"replicated"`
	Counter      *int64  `json:"counter"`
	CounterA     *int64  `json:"counter_a"`
	CounterB     *int64  `json:"counter_b"`
	CtnHighA     *int64  `json:"ctn_high_a"`
	CtnLowA      *int64  `json:"ctn_low_a"`
	CtnHighB     *int64  `json:"ctn_high_b"`
	CtnLowB      *int64  `json:"ctn_low_b"`
}

type TTAgentXlaBookmark struct {
	WriteLfn int64 `json:"writelfn"`
	WriteLfo int64 `json:"writelfo"`
	ForceLfn int64 `json:"forcelfn"`
	ForceLfo int64 `json:"forcelfo"`
	HoldLfn  int64 `json:"holdlfn"`
	HoldLfo  int64 `json:"holdlfo"`
}

type TTAgentXlaInfo struct {
	Subscriptions     []TTAgentXlaSubscription `json:"subscriptions,omitempty"`
	TransactionLogApi []TTAgentXla             `json:"transaction_log_api,omitempty"`
	Bookmark          []TTAgentXlaBookmark     `json:"bookmark,omitempty"`
}

// This is a Go definition of the JSON that "get1.sql" prints out

type TTAgentOut struct {
	Id                    int64                                `json:"id"`
	JsonVer               int                                  `json:"jsonVer"`
	Errno                 int                                  `json:"errno"`
	Errmsg                string                               `json:"errmsg,omitempty"`
	ClockSync             *bool                                `json:"clockSync"`
	DaemonUp              bool                                 `json:"daemonUp"`
	InstanceExists        *bool                                `json:"instanceExists"`
	InstallRelease        string                               `json:"installRelease"`
	ImageRelease          string                               `json:"imageRelease"`
	TTStatus              timestenv2.TTStatus                  `json:"ttStatus"`
	DbUp                  bool                                 `json:"dbUp"`
	DbError               string                               `json:"dbError"`
	RepState              string                               `json:"repState"`
	NRepSchemes           int                                  `json:"nRepSchemes"`
	BehindMB              int                                  `json:"behindMb"`
	BehindFiles           int                                  `json:"behindFiles"`
	LogFileSize           int                                  `json:"logFileSize"`
	Updatable             bool                                 `json:"updatable"`
	NewId                 int64                                `json:"newId"`
	UpdateErr             string                               `json:"updateErr"`
	RepPeerPState         string                               `json:"repPeerPState"`
	RepPeerPStateFetchErr string                               `json:"repPeerPStateFetchErr,omitempty"`
	NCacheGroups          int                                  `json:"nCacheGroups"`
	CacheUidPwdSet        bool                                 `json:"cacheUidPwdSet"`
	AdminUserFile         bool                                 `json:"adminUserFile"`           // Does the adminUser file exist?
	SchemaFile            bool                                 `json:"schemaFile"`              // Does the schema.sql file exist?
	CacheGroupsFile       bool                                 `json:"cgFile"`                  // Does the cachegroups.sql file exist?
	CacheUserFile         bool                                 `json:"cacheUserFile"`           // Does the cacheUser file exist?
	UsingTwosafe          *bool                                `json:"usingTwosafe,omitempty"`  // Is the repscheme TWOSAFE?
	DisableReturn         *bool                                `json:"disableReturn,omitempty"` // Does repscheme specify DISABLE RETURN?
	LocalCommit           *bool                                `json:"localCommit,omitempty"`   // Does repscheme specify LOCAL COMMIT?
	AwtBehindMb           *float32                             `json:"awtBehindMb,omitempty"`   // How far behind is AWT (if in use)? //SAMDRAKE PUT BACK PLEEZE
	AwtBehindErr          *string                              `json:"awtBehindErr,omitempty"`  // Error if we couldn't fetch AwtBehindMb
	BookmarkLFN           *int64                               `json:"bookmarkLfn,omitempty"`
	BookmarkLFO           *int64                               `json:"bookmarkLfo,omitempty"`
	LogHoldsLFN           *int64                               `json:"logholdsLfn,omitempty"`
	LogHoldsLFO           *int64                               `json:"logholdsLfo,omitempty"`
	UpgradeList           *string                              `json:"upgradeList,omitempty"` // do not omit if empty, we want to know
	InstanceType          string                               `json:"instanceType"`
	Monitor               *map[string]string                   `json:"monitor,omitempty"`
	SystemStats           *map[string]string                   `json:"systemStats,omitempty"`
	Configuration         *map[string]string                   `json:"configuration,omitempty"` // sys.v$configuration from the db if any
	RepStats              []timestenv2.TimesTenReplicationStat `json:"repStats,omitempty"`
	GridDbStatus          *timestenv2.ScaleoutDbStatus         `json:"gridDbStatus,omitempty"`    // Mgmt instances only
	GridMgmtExamine       *timestenv2.ScaleoutMgmtExamine      `json:"gridMgmtExamine,omitempty"` // Mgmt instances only
	XLA                   interface{}                          `json:"xla,omitempty"`
	TTDiskAll             *uint64                              `json:"ttDiskAll,omitempty"`
	TTDiskAvail           *uint64                              `json:"ttDiskAvail,omitempty"`
	TTDiskFree            *uint64                              `json:"ttDiskFree,omitempty"`
	TTLogAll              *uint64                              `json:"ttLogAll,omitempty"`
	TTLogAvail            *uint64                              `json:"ttLogAvail,omitempty"`
	TTLogFree             *uint64                              `json:"ttLogFree,omitempty"`
	CGroupInfo            *timestenv2.CGroupMemoryInfo         `json:"cgroupInfo,omitempty"`
	Cachegroups           *[]timestenv2.CacheGroupStatusType   `json:"cachegroups,omitempty"`
	Stderr                []string                             `json:"stderr,omitempty"`
	Quiescing             bool                                 `json:"quiescing,omitempty"`
	NonRepUpgradeFailed   bool                                 `json:"nonRepUpgradeFailed,omitempty"`
}

type AsyncTask struct {
	JsonVer             *int                  `json:"jsonVer"`
	Errno               *int                  `json:"errno"`
	Errmsg              *string               `json:"errmsg"`
	Id                  string                `json:"id"`
	Type                string                `json:"type"`
	Running             bool                  `json:"running"`
	Complete            bool                  `json:"complete"`
	Updated             *int64                `json:"updated,omitempty"` // time.Unix()
	Started             *int64                `json:"started,omitempty"` // time.Unix()
	Ended               *int64                `json:"ended,omitempty"`   // time.Unix()
	AgentDuplicateReply TTAgentDuplicateReply `json:"dup,omitempty"`
	AgentCreateCgReply  TTAgentCreateCgReply  `json:"cg,omitempty"`
}

//----------------------------------------------------------------------
// Output from the Agent from POST requests
//----------------------------------------------------------------------

// Common JSON datum returned by the Agent from EVERY request
type TTAgentPostReply struct {
	JsonVer *int    `json:"jsonVer"`
	Errno   *int    `json:"errno"`
	Errmsg  *string `json:"errmsg"`
}

// Generic results from the Agent after it runs an OS command
type TTAgentGenericReply struct {
	JsonVer *int      `json:"jsonVer"`
	Errno   *int      `json:"errno"`
	Errmsg  *string   `json:"errmsg"`
	Stdout  *[]string `json:"stdout"`
	Stderr  *[]string `json:"stderr"`
}

// The JSON returned by the Agent from a 'getDbShmRequirement' request
type TTAgentShmRequirementReply struct {
	JsonVer *int      `json:"jsonVer"`
	Errno   *int      `json:"errno,omitifempty"`
	Errmsg  *string   `json:"errmsg,omitifempty"`
	Stdout  *[]string `json:"stdout,omitifempty"`
	Stderr  *[]string `json:"stderr,omitifempty"`
	ShmSize *int64    `json:"shmSize,omitifempty"`
}

// The JSON returned by the Agent from a 'doCheckQuiesce' request
type TTAgentDoCheckQuiesce struct {
	JsonVer   *int      `json:"jsonVer"`
	Errno     *int      `json:"errno,omitifempty"`
	Errmsg    *string   `json:"errmsg,omitifempty"`
	Stdout    *[]string `json:"stdout,omitifempty"`
	Stderr    *[]string `json:"stderr,omitifempty"`
	Quiescing *bool     `json:"quiescing,omitifempty"`
}

// The JSON returned by the Agent from a 'removeNonRepUpgradeFailedFile' request
type TTAgentRemoveFile struct {
	JsonVer *int      `json:"jsonVer"`
	Errno   *int      `json:"errno,omitifempty"`
	Errmsg  *string   `json:"errmsg,omitifempty"`
	Stdout  *[]string `json:"stdout,omitifempty"`
	Stderr  *[]string `json:"stderr,omitifempty"`
	Removed *bool     `json:"removed,omitifempty"`
}

// The JSON returned by the Agent from a 'createDb' request
type TTAgentCreateDbReply struct {
	JsonVer              *int     `json:"jsonVer"`
	Errno                *int     `json:"errno"`
	Errmsg               *string  `json:"errmsg,omitempty"`
	CreatedDatabase      *bool    `json:"createdDatabase"`
	CreatedOurAdminUser  *bool    `json:"createdOurAdminUser"`
	CreatedUserAdminUser *bool    `json:"createdUserAdminUser"`
	CreatedUserTestUser  *bool    `json:"createdUserTestUser"`
	UsingCache           *bool    `json:"usingCache"`
	ReadCacheUserFile    *bool    `json:"readCacheUserFile"`
	ReadAdminUserFile    *bool    `json:"readAdminUserFile"`
	SchemaFileExists     *bool    `json:"schemaFileExists"`
	SchemaFileRc         *int     `json:"schemaFileRc,omitempty"`
	CreatedCacheUser     *bool    `json:"createdCacheUser"`
	SetCacheUidPwd       *bool    `json:"setCacheUidPwdSet"`
	Errors               []string `json:"errors,omitempty"`
	Create1Out           []string `json:"create1Out,omitempty"`
	Create1Err           []string `json:"create1Err,omitempty"`
	Create2Out           []string `json:"create2Out,omitempty"`
	Create2Err           []string `json:"create2Err,omitempty"`
	SchemaOut            []string `json:"schemaOut,omitempty"`
	SchemaErr            []string `json:"schemaErr,omitempty"`
	DebugMsgs            []string `json:"debugMsgs,omitempty"`
}

// The JSON returned by the Agent from a 'createCg' request
type TTAgentCreateCgReply struct {
	JsonVer           int       `json:"jsonVer"`
	Errno             int       `json:"errno"`
	ReadCacheUserFile *bool     `json:"readCacheUserFile"`
	CgFileExists      *bool     `json:"cgFileExists"`
	CgFileRc          *int      `json:"cgFileRc,omitempty"`
	CgOut             *[]string `json:"cgout,omitempty"`
	CgErr             *[]string `json:"cgErr,omitempty"`
	DebugMsgs         *[]string `json:"debugMsgs,omitempty"`
	Errors            []string  `json:"errors"`
}

// The JSON returned by the Agent from a 'destroyDb' request
type TTAgentDestroyDbReply struct {
	JsonVer    int      `json:"jsonVer"`
	RepRc      *int     `json:"repRc"`
	RepOut     []string `json:"repOut"`
	CacRc      *int     `json:"cacRc"`
	CacOut     []string `json:"cacOut"`
	CloseRc    *int     `json:"closeRc"`
	CloseOut   []string `json:"closeOut"`
	Unl1Rc     *int     `json:"unl1Rc"`
	Unl1Out    []string `json:"unl1Out"`
	Unl2Rc     *int     `json:"unl2Rc"`
	Unl2Out    []string `json:"unl2Out"`
	DiscRc     *int     `json:"discRc"`
	DiscOut    []string `json:"discOut"`
	Errno      *int     `json:"errno"` // This is actually the rc from the destroy
	DestroyOut []string `json:"destroyOut"`
}

// The JSON returned by the Agent from a 'doCleanupCache' request
type DoCleanupCacheReply struct {
	JsonVer int    `json:"jsonVer"`
	Msg     string `json:"msg"`
	Errno   *int   `json:"errno"` // requester should check this value
}

// The JSON returned by the Agent from a 'openDb' request
type TTAgentOpenDbReply struct {
	JsonVer int       `json:"jsonVer"`
	CmdRc   *int      `json:"cmdRc"`
	CmdOut  *[]string `json:"cmdOut"`
	Msg     string    `json:"msg"`
	Errno   *int      `json:"errno"`
}

// The JSON returned by the Agent from a 'closeDb' request
type TTAgentCloseDbReply struct {
	JsonVer int       `json:"jsonVer"`
	CmdRc   *int      `json:"cmdRc"`
	CmdOut  *[]string `json:"cmdOut"`
	Msg     string    `json:"msg"`
	Errno   *int      `json:"errno"`
}

// The JSON returned by the Agent from a 'force disconnect' request
type TTAgentForceDisconnect struct {
	JsonVer int       `json:"jsonVer"`
	CmdRc   *int      `json:"cmdRc"`
	Stdout  *[]string `json:"stdout"`
	Stderr  *[]string `json:"stderr"`
	Msg     string    `json:"msg"`
	Errno   *int      `json:"errno"`
}

// The JSON returned by the Agent from a 'doRepSubscriberWait' request
type TTAgentRepSubscriberWaitReply struct {
	JsonVer int       `json:"jsonVer"`
	Results *string   `json:"results"`
	Errno   *int      `json:"errno"`
	Errmsg  *string   `json:"errmsg"`
	Stdout  *[]string `json:"stdout"`
	Stderr  *[]string `json:"stderr"`
}

// Reply from Agent when duplicating database
type TTAgentDuplicateReply struct {
	JsonVer   int      `json:"jsonVer"`
	Errno     int      `json:"errno"`
	DupRc     int      `json:"dupRc"`
	DupOut    []string `json:"dupOut"`
	DupErr    []string `json:"dupErr"`
	CsOut     []string `json:"csOut"`
	CsErr     []string `json:"csErr"`
	PolicyOut []string `json:"policyOut"`
	PolicyErr []string `json:"policyErr"`
	Errors    []string `json:"errors"`
}

// Reply from Agent when calling ttRepAdmin -wait
type TTAgentRepAdminWaitReply struct {
	JsonVer *int      `json:"jsonVer"`
	Rc      *int      `json:"Rc"`
	Errno   *int      `json:"Errno"`
	Errmsg  *string   `json:"Errmsg"`
	Stdout  *[]string `json:"Stdout"`
	Stderr  *[]string `json:"Stderr"`
}

// maps to upgrade.json schemaVersion 1
type upgradeCompat_v1 struct {
	What          string            `json:"what"`
	Version       int               `json:"ver"`
	Created       string            `json:"created"`
	ValidUpgrades []upgradeEntry_v1 `json:"validUpgrades"`
}

// maps to upgrade.json schemaVersion 2
type upgradeCompat_v2 struct {
	What          string            `json:"what"`
	Version       int               `json:"ver"`
	Created       string            `json:"created"`
	ValidUpgrades []upgradeEntry_v2 `json:"validUpgrades"`
}

// maps to upgrade.json schemaVersion 1
type upgradeEntry_v1 struct {
	From     string             `json:"from"`
	To       string             `json:"to"`
	Classic  *map[string]string `json:"classic"`
	Scaleout *map[string]string `json:"scaleout"`
}

// maps to upgrade.json schemaVersion 2
type upgradeEntry_v2 struct {
	From     string       `json:"from"`
	To       string       `json:"to"`
	Classic  *ClassicUpg  `json:"classic"`
	Scaleout *ScaleoutUpg `json:"scaleout"`
}

// maps to ALL upgrade.json schemaVersions
type ClassicUpg struct {
	Offline *OfflineUpg `json:"offline"`
	Online  *OnlineUpg  `json:"online"`
}

// maps to ALL upgrade.json schemaVersions
type ScaleoutUpg struct {
	Offline *OfflineUpg `json:"offline"`
	Online  *OnlineUpg  `json:"online"`
}

// maps to upgrade.json schemaVersions > 1
type OfflineUpg struct {
	Inverse *bool `json:"inverse"`
}

// maps to ALL upgrade.json schemaVersions > 1
type OnlineUpg struct {
	Inverse *bool `json:"inverse"`
}

/* Emacs variable settings */
/* Local Variables: */
/* tab-width:4 */
/* indent-tabs-mode:nil */
/* End: */
