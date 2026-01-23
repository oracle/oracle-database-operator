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

// getRacDbModeCmd provides documentation for the getRacDbModeCmd function.
func getRacDbModeCmd() []string {
	oraScriptMount1 := getOraScriptMount()
	oraPythonCmd := getOraPythonCmd()
	var oraRacInstCmd = []string{oraScriptMount1 + "/cmdExec", oraPythonCmd, oraScriptMount1 + "/main.py ", "--checkracinst=true "}
	return oraRacInstCmd
}

// getGiHealthCmd provides documentation for the getGiHealthCmd function.
func getGiHealthCmd() []string {
	oraScriptMount1 := getOraScriptMount()
	oraPythonCmd := getOraPythonCmd()
	var oraGiCmd = []string{oraScriptMount1 + "/cmdExec", oraPythonCmd, oraScriptMount1 + "/main.py ", "--checkgilocal=true "}
	return oraGiCmd
}

// getRACHealthCmd provides documentation for the getRACHealthCmd function.
func getRACHealthCmd() []string {
	oraScriptMount1 := getOraScriptMount()
	oraPythonCmd := getOraPythonCmd()
	var oraRacCmd = []string{oraScriptMount1 + "/cmdExec", oraPythonCmd, oraScriptMount1 + "/main.py ", "--checkracdb=true "}
	return oraRacCmd
}

// getConnStrCmd provides documentation for the getConnStrCmd function.
func getConnStrCmd() []string {
	oraScriptMount1 := getOraScriptMount()
	oraPythonCmd := getOraPythonCmd()
	var oraRacConnCmd = []string{oraScriptMount1 + "/cmdExec", oraPythonCmd, oraScriptMount1 + "/main.py ", "--checkconnstr=true "}
	return oraRacConnCmd
}

// getGridHomeCmd provides documentation for the getGridHomeCmd function.
func getGridHomeCmd() []string {
	// Command to source the envfile and echo GRID_HOME
	gridHomeCmd := []string{"sh", "-c", "grep '^GRID_HOME=' /etc/rac_env_vars/envfile | cut -d'=' -f2"}
	return gridHomeCmd
}

// getPdbConnStrCmd provides documentation for the getPdbConnStrCmd function.
func getPdbConnStrCmd() []string {
	oraScriptMount1 := getOraScriptMount()
	oraPythonCmd := getOraPythonCmd()
	var oraRacConnCmd = []string{oraScriptMount1 + "/cmdExec", oraPythonCmd, oraScriptMount1 + "/main.py ", "--checkpdbconnstr=true "}
	return oraRacConnCmd
}

// getDbRoleCmd provides documentation for the getDbRoleCmd function.
func getDbRoleCmd() []string {
	oraScriptMount1 := getOraScriptMount()
	oraPythonCmd := getOraPythonCmd()
	var oraRacRoleCmd = []string{oraScriptMount1 + "/cmdExec", oraPythonCmd, oraScriptMount1 + "/main.py ", "--checkdbrole=true "}
	return oraRacRoleCmd
}

// getRacDbVersionCmd provides documentation for the getRacDbVersionCmd function.
func getRacDbVersionCmd() []string {
	oraScriptMount1 := getOraScriptMount()
	oraPythonCmd := getOraPythonCmd()
	var oraRacVersionCmd = []string{oraScriptMount1 + "/cmdExec", oraPythonCmd, oraScriptMount1 + "/main.py ", "--checkdbversion=true "}
	return oraRacVersionCmd
}

// getDBServiceStatus provides documentation for the getDBServiceStatus function.
func getDBServiceStatus(dbhome string, dbname string, svcname string) []string {
	oraScriptMount1 := getOraScriptMount()
	oraPythonCmd := getOraPythonCmd()
	svcStr := "\"" + "service=" + svcname + ";dbname=" + dbname + "\""
	var oraRacVersionCmd = []string{oraScriptMount1 + "/cmdExec", oraPythonCmd, oraScriptMount1 + "/main.py ", "--checkdbsvc=" + svcStr}
	return oraRacVersionCmd
}

// modifyDBServiceStatus provides documentation for the modifyDBServiceStatus function.
func modifyDBServiceStatus(dbhome string, dbname string, svcname string) []string {
	oraDBUser := getOraDbUser()

	//oraGiUser := getOraGiUser()
	svcModifyCmd := "su " + oraDBUser + " -c \"" + oraDBUser + " ;srvctl modify service -s " + svcname + " -d " + dbname + " " + dbhome + "\""
	var oraModifySvcCmd = []string{svcModifyCmd}
	return oraModifySvcCmd
}

// getAsmDiskgroupCmd provides documentation for the getAsmDiskgroupCmd function.
func getAsmDiskgroupCmd() []string {
	oraScriptMount1 := getOraScriptMount()
	oraPythonCmd := getOraPythonCmd()
	diskgroups := []string{oraScriptMount1 + "/cmdExec", oraPythonCmd, oraScriptMount1 + "/main.py ", "--getasmdiskgroup=true"}
	return diskgroups
}

// getAsmDisksCmd provides documentation for the getAsmDisksCmd function.
func getAsmDisksCmd(diskgroup string) []string {
	oraScriptMount1 := getOraScriptMount()
	oraPythonCmd := getOraPythonCmd()
	disks := []string{oraScriptMount1 + "/cmdExec", oraPythonCmd, oraScriptMount1 + "/main.py ", "--getasmdisks=" + diskgroup}
	return disks
}

// getAsmDgRedundancyCmd provides documentation for the getAsmDgRedundancyCmd function.
func getAsmDgRedundancyCmd(diskgroup string) []string {
	oraScriptMount1 := getOraScriptMount()
	oraPythonCmd := getOraPythonCmd()
	redundancy := []string{oraScriptMount1 + "/cmdExec", oraPythonCmd, oraScriptMount1 + "/main.py ", "--getdgredundancy=" + diskgroup}
	return redundancy
}

// getAsmInstNameCmd provides documentation for the getAsmInstNameCmd function.
func getAsmInstNameCmd() []string {
	oraScriptMount1 := getOraScriptMount()
	oraPythonCmd := getOraPythonCmd()
	name := []string{oraScriptMount1 + "/cmdExec", oraPythonCmd, oraScriptMount1 + "/main.py ", "--getasminstname=true"}
	return name
}

// getAsmInstStatusCmd provides documentation for the getAsmInstStatusCmd function.
func getAsmInstStatusCmd() []string {
	oraScriptMount1 := getOraScriptMount()
	oraPythonCmd := getOraPythonCmd()
	status := []string{oraScriptMount1 + "/cmdExec", oraPythonCmd, oraScriptMount1 + "/main.py ", "--getasminststatus=true"}
	return status
}

// This function generate the CMD for updating the end points of the Scan
// getUpdateScanEpCmd provides documentation for the getUpdateScanEpCmd function.
func getUpdateScanEpCmd(gihome string, scanname string) []string {
	oraScriptMount1 := getOraScriptMount()
	oraPythonCmd := getOraPythonCmd()
	var oraUpdScanEpCmd = []string{oraScriptMount1 + "/cmdExec", oraPythonCmd, oraScriptMount1 + "/main.py ", "--modifyscan=" + scanname}
	return oraUpdScanEpCmd
}

// This function generates the CMD for reconciling CDP configuration
// getUpdateCdpCmd provides documentation for the getUpdateCdpCmd function.
func getUpdateCdpCmd(gihome string) []string {
	oraScriptMount1 := getOraScriptMount()
	oraPythonCmd := getOraPythonCmd()

	var oraUpdCdpCmd = []string{
		oraScriptMount1 + "/cmdExec",
		oraPythonCmd,
		oraScriptMount1 + "/main.py",
		"--updatecdp=True",
	}

	return oraUpdCdpCmd
}

// This function generates command for updating the TCP listener endpoints
// getUpdateTCPPortCmd provides documentation for the getUpdateTCPPortCmd function.
func getUpdateTCPPortCmd(gihome string, portlist string, lsnrname string) []string {
	oraScriptMount1 := getOraScriptMount()
	oraPythonCmd := getOraPythonCmd()
	endp := "lsnrname=" + lsnrname + ";" + "portlist=" + portlist
	var oraUpdTCPPortCmd = []string{oraScriptMount1 + "/cmdExec", oraPythonCmd, oraScriptMount1 + "/main.py ", "--updatelsnrendp=" + "\"" + endp + "\""}
	return oraUpdTCPPortCmd
}

// This function generates cmmand for modifying asm cardinality for the nodes
// getUpdateAsmCount provides documentation for the getUpdateAsmCount function.
func getUpdateAsmCount(gihome string) []string {
	oraScriptMount1 := getOraScriptMount()
	oraPythonCmd := getOraPythonCmd()
	var oraUpdAsmCountCmd = []string{oraScriptMount1 + "/cmdExec", oraPythonCmd, oraScriptMount1 + "/main.py ", "--updateasmcount=2"}
	return oraUpdAsmCountCmd
}

// This function generates cmmand for modifying asm cardinality for the nodes
// getRacInstStateFileCmd provides documentation for the getRacInstStateFileCmd function.
func getRacInstStateFileCmd() []string {
	var oraStateCmd = []string{"/bin/bash", "-c", "  cat /tmp/orod/.statefile"}
	return oraStateCmd
}
