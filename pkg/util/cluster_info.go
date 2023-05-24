package util

import "sync"

type ESXiVersionInfo struct {
	Version    string
	APIVersion string
}

type ClusterInfo struct {
	// map of host and its esxi version info
	esxiVersions      map[string]ESXiVersionInfo
	hwVersions        map[string]int
	cbtEnabled        map[string]int
	vcenterVersion    string
	vcenterAPIVersion string
	esxiVersionsLock  sync.RWMutex
}

func NewClusterInfo() *ClusterInfo {
	info := &ClusterInfo{
		esxiVersions: make(map[string]ESXiVersionInfo),
		hwVersions:   make(map[string]int),
		cbtEnabled:   make(map[string]int),
	}
	return info
}

// MakeClusterInfo is only used for tests
func MakeClusterInfo(d map[string]string) *ClusterInfo {
	info := &ClusterInfo{
		esxiVersions: make(map[string]ESXiVersionInfo),
		hwVersions:   make(map[string]int),
	}
	info.esxiVersions[d["host_name"]] = ESXiVersionInfo{d["host_version"], d["host_api_version"]}
	info.hwVersions[d["hw_version"]] = 1
	info.vcenterAPIVersion = d["vcenter_api_version"]
	info.vcenterVersion = d["vcenter_version"]
	return info
}

func (c *ClusterInfo) SetHostVersion(hostname, version, apiVersion string) {
	c.esxiVersionsLock.Lock()
	defer c.esxiVersionsLock.Unlock()

	c.esxiVersions[hostname] = ESXiVersionInfo{version, apiVersion}
}

func (c *ClusterInfo) SetHardwareVersion(version string) {
	c.esxiVersionsLock.Lock()
	defer c.esxiVersionsLock.Unlock()

	c.hwVersions[version]++
}

func (c *ClusterInfo) GetHardwareVersion() map[string]int {
	c.esxiVersionsLock.RLock()
	defer c.esxiVersionsLock.RUnlock()

	// make a copy
	hwVersions := map[string]int{}
	for h, v := range c.hwVersions {
		hwVersions[h] = v
	}
	return hwVersions
}

func (c *ClusterInfo) SetVCenterVersion(version, apiVersion string) {
	c.esxiVersionsLock.Lock()
	defer c.esxiVersionsLock.Unlock()
	c.vcenterVersion = version
	c.vcenterAPIVersion = apiVersion
}

func (c *ClusterInfo) GetHostVersions() map[string]ESXiVersionInfo {
	c.esxiVersionsLock.RLock()
	defer c.esxiVersionsLock.RUnlock()

	// make a copy of return values
	hostVersions := make(map[string]ESXiVersionInfo)
	for h, v := range c.esxiVersions {
		hostVersions[h] = v
	}
	return hostVersions
}

func (c *ClusterInfo) GetVCenterVersion() (string, string) {
	c.esxiVersionsLock.RLock()
	defer c.esxiVersionsLock.RUnlock()
	return c.vcenterVersion, c.vcenterAPIVersion
}

func (c *ClusterInfo) Reset() {
	c.esxiVersionsLock.Lock()
	defer c.esxiVersionsLock.Unlock()
	c.esxiVersions = make(map[string]ESXiVersionInfo)
	c.hwVersions = make(map[string]int)
	c.vcenterVersion = ""
	c.vcenterAPIVersion = ""
}

func (c *ClusterInfo) MarkHostForProcessing(hostname string) (string, bool) {
	c.esxiVersionsLock.Lock()
	defer c.esxiVersionsLock.Unlock()

	var esxiVersion string
	ver, found := c.esxiVersions[hostname]
	if ver.Version == "" {
		esxiVersion = "<in progress>"
	}
	if found {
		return ver.Version, true
	}
	// Mark the hostName as in progress
	c.esxiVersions[hostname] = ESXiVersionInfo{"", ""}
	return esxiVersion, false
}

// SetCbtData Set a node as being enabled or disabled for CBT
func (c *ClusterInfo) SetCbtData(enabled string) {
	c.esxiVersionsLock.RLock()
	defer c.esxiVersionsLock.RUnlock()

	c.cbtEnabled[enabled]++
}

// GetCbtData Get the CBT enabled settings for vms.  This will be a count of how
// many VMs are enabled (true) and how many are disabled (false).
func (c *ClusterInfo) GetCbtData() map[string]int {
	c.esxiVersionsLock.RLock()
	defer c.esxiVersionsLock.RUnlock()

	// Make a copy
	cbtEnabled := map[string]int{}
	for h, v := range c.cbtEnabled {
		cbtEnabled[h] = v
	}
	return cbtEnabled
}
