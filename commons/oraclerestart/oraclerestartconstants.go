/*
** Copyright (c) 2022 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
**
** Subject to the condition set forth below, permission is hereby granted to any
** person obtaining a copy of this software, associated documentation and/or data
** (collectively the "Software"), free of charge and under any and all copyright
** rights in the Software, and any and all patent rights owned or freely
** licensable by each licensor hereunder covering either (i) the unmodified
** Software as contributed to or provided by such licensor, or (ii) the Larger
** Works (as defined below), to deal in both
**
** (a) the Software, and
** (b) any piece of software and/or hardware listed in the lrgrwrks.txt file if
** one is included with the Software (each a "Larger Work" to which the Software
** is contributed by such licensors),
**
** without restriction, including without limitation the rights to copy, create
** derivative works of, display, perform, and distribute the Software and make,
** use, sell, offer for sale, import, export, have made, and have sold the
** Software and the Larger Work(s), and to sublicense the foregoing rights on
** either these or other terms.
**
** This license is subject to the following condition:
** The above copyright notice and either this complete permission notice or at
** a minimum a reference to the UPL must be included in all copies or
** substantial portions of the Software.
**
** THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
** IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
** FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
** AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
** LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
** OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
** SOFTWARE.
 */

package commons

func getOracleRestartDbModeCmd() []string {
	oraScriptMount1 := getOraScriptMount()
	oraPythonCmd := getOraPythonCmd()
	var oraRacInstCmd = []string{oraScriptMount1 + "/cmdExec", oraPythonCmd, oraScriptMount1 + "/main.py ", "--checkracinst=true "}
	return oraRacInstCmd
}

func getGiHealthCmd() []string {
	oraScriptMount1 := getOraScriptMount()
	oraPythonCmd := getOraPythonCmd()
	var oraGiCmd = []string{oraScriptMount1 + "/cmdExec", oraPythonCmd, oraScriptMount1 + "/main.py ", "--checkgilocal=true "}
	return oraGiCmd
}

func getOracleRestartHealthCmd() []string {
	oraScriptMount1 := getOraScriptMount()
	oraPythonCmd := getOraPythonCmd()
	var oraRacCmd = []string{oraScriptMount1 + "/cmdExec", oraPythonCmd, oraScriptMount1 + "/main.py ", "--checkracdb=true "}
	return oraRacCmd
}

func getConnStrCmd() []string {
	oraScriptMount1 := getOraScriptMount()
	oraPythonCmd := getOraPythonCmd()
	var oraRacConnCmd = []string{oraScriptMount1 + "/cmdExec", oraPythonCmd, oraScriptMount1 + "/main.py ", "--checkconnstr=true "}
	return oraRacConnCmd
}
func getGridHomeCmd() []string {
	// Command to source the envfile and echo GRID_HOME
	gridHomeCmd := []string{"sh", "-c", "grep '^GRID_HOME=' /etc/rac_env_vars/envfile | cut -d'=' -f2"}
	return gridHomeCmd
}

func getPdbConnStrCmd() []string {
	oraScriptMount1 := getOraScriptMount()
	oraPythonCmd := getOraPythonCmd()
	var oraRacConnCmd = []string{oraScriptMount1 + "/cmdExec", oraPythonCmd, oraScriptMount1 + "/main.py ", "--checkpdbconnstr=true "}
	return oraRacConnCmd
}

func getDbRoleCmd() []string {
	oraScriptMount1 := getOraScriptMount()
	oraPythonCmd := getOraPythonCmd()
	var oraRacRoleCmd = []string{oraScriptMount1 + "/cmdExec", oraPythonCmd, oraScriptMount1 + "/main.py ", "--checkdbrole=true "}
	return oraRacRoleCmd
}

func getOracleRestartDbVersionCmd() []string {
	oraScriptMount1 := getOraScriptMount()
	oraPythonCmd := getOraPythonCmd()
	var oraRacVersionCmd = []string{oraScriptMount1 + "/cmdExec", oraPythonCmd, oraScriptMount1 + "/main.py ", "--checkdbversion=true "}
	return oraRacVersionCmd
}

func getDBServiceStatus(dbhome string, dbname string, svcname string) []string {
	oraScriptMount1 := getOraScriptMount()
	oraPythonCmd := getOraPythonCmd()
	svcStr := "\"" + "service=" + svcname + ";dbname=" + dbname + "\""
	var oraRacVersionCmd = []string{oraScriptMount1 + "/cmdExec", oraPythonCmd, oraScriptMount1 + "/main.py ", "--checkdbsvc=" + svcStr}
	return oraRacVersionCmd
}

func modifyDBServiceStatus(dbhome string, dbname string, svcname string) []string {
	oraDBUser := getOraDbUser()

	//oraGiUser := getOraGiUser()
	svcModifyCmd := "su " + oraDBUser + " -c \"" + oraDBUser + " ;srvctl modify service -s " + svcname + " -d " + dbname + " " + dbhome + "\""
	var oraModifySvcCmd = []string{svcModifyCmd}
	return oraModifySvcCmd
}

func getAsmDiskgroupCmd() []string {
	oraScriptMount1 := getOraScriptMount()
	oraPythonCmd := getOraPythonCmd()
	diskgroupscmd := []string{oraScriptMount1 + "/cmdExec", oraPythonCmd, oraScriptMount1 + "/main.py ", "--getasmdiskgroup=true"}
	return diskgroupscmd
}

func getAsmDisksCmd(diskgroup string) []string {
	oraScriptMount1 := getOraScriptMount()
	oraPythonCmd := getOraPythonCmd()
	disks := []string{oraScriptMount1 + "/cmdExec", oraPythonCmd, oraScriptMount1 + "/main.py ", "--getasmdisks=" + diskgroup}
	return disks
}

func getAsmDgRedundancyCmd(diskgroup string) []string {
	oraScriptMount1 := getOraScriptMount()
	oraPythonCmd := getOraPythonCmd()
	redundancy := []string{oraScriptMount1 + "/cmdExec", oraPythonCmd, oraScriptMount1 + "/main.py ", "--getdgredundancy=" + diskgroup}
	return redundancy
}

func getAsmInstNameCmd() []string {
	oraScriptMount1 := getOraScriptMount()
	oraPythonCmd := getOraPythonCmd()
	name := []string{oraScriptMount1 + "/cmdExec", oraPythonCmd, oraScriptMount1 + "/main.py ", "--getasminstname=true"}
	return name
}

func getAsmInstStatusCmd() []string {
	oraScriptMount1 := getOraScriptMount()
	oraPythonCmd := getOraPythonCmd()
	status := []string{oraScriptMount1 + "/cmdExec", oraPythonCmd, oraScriptMount1 + "/main.py ", "--getasminststatus=true"}
	return status
}

// This function generates cmmand for modifying asm cardinality for the nodes
func getUpdateAsmCount(gihome string) []string {
	oraScriptMount1 := getOraScriptMount()
	oraPythonCmd := getOraPythonCmd()
	var oraUpdAsmCountCmd = []string{oraScriptMount1 + "/cmdExec", oraPythonCmd, oraScriptMount1 + "/main.py ", "--updateasmcount=2"}
	return oraUpdAsmCountCmd
}

// This function generates cmmand for modifying asm cardinality for the nodes
func getOracleRestartInstStateFileCmd() []string {
	var oraStateCmd = []string{"/bin/bash", "-c", "  cat /tmp/orod/.statefile"}
	return oraStateCmd
}
