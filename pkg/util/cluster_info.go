package util

import "sync"

var (
	VSphereClusterInfo *ClusterInfo
	once               sync.Once
)

func init() {
	once.Do(func() {
		VSphereClusterInfo = &ClusterInfo{
			esxiVersions: make(map[string]EsxiVersionInfo),
			hwVersions:   make(map[string]int),
		}
	})
}

type EsxiVersionInfo struct {
	Version    string
	ApiVersion string
}

type ClusterInfo struct {
	// map of host and its esxi version info
	esxiVersions      map[string]EsxiVersionInfo
	hwVersions        map[string]int
	vcenterVersion    string
	vcenterAPIVersion string
	esxiVersionsLock  sync.RWMutex
}

func MakeClusterInfo(d map[string]string) *ClusterInfo {
	info := &ClusterInfo{
		esxiVersions: make(map[string]EsxiVersionInfo),
		hwVersions:   make(map[string]int),
	}
	info.esxiVersions[d["host_name"]] = EsxiVersionInfo{d["host_version"], d["host_api_version"]}
	info.hwVersions[d["hw_version"]] = 1
	info.vcenterAPIVersion = d["vcenter_api_version"]
	info.vcenterVersion = d["vcenter_version"]
	return info
}

func (c *ClusterInfo) SetHostVersion(hostname, version, apiVersion string) {
	c.esxiVersionsLock.Lock()
	defer c.esxiVersionsLock.Unlock()

	c.esxiVersions[hostname] = EsxiVersionInfo{version, apiVersion}
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

func (c *ClusterInfo) GetHostVersions() map[string]EsxiVersionInfo {
	c.esxiVersionsLock.RLock()
	defer c.esxiVersionsLock.RUnlock()

	// make a copy of return values
	hostVersions := make(map[string]EsxiVersionInfo)
	for h, v := range c.esxiVersions {
		hostVersions[h] = v
	}
	return hostVersions
}

func (c *ClusterInfo) GetVCenterInfo() (string, string) {
	c.esxiVersionsLock.RLock()
	defer c.esxiVersionsLock.RUnlock()
	return c.vcenterVersion, c.vcenterAPIVersion
}

func (c *ClusterInfo) Reset() {
	c.esxiVersionsLock.Lock()
	defer c.esxiVersionsLock.Unlock()
	c.esxiVersions = make(map[string]EsxiVersionInfo)
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
	c.esxiVersions[hostname] = EsxiVersionInfo{"", ""}
	return esxiVersion, false
}
