package kubernetes

import (
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/request"

	api "k8s.io/api/core/v1"
)

// AutoPath implements the AutoPathFunc call from the autopath plugin.
// It returns a per-query search path or nil indicating no searchpathing should happen.
func (k *Kubernetes) AutoPath(state request.Request) []string {
	// Check if the query falls in a zone we are actually authoritative for and thus if we want autopath.
	zone := plugin.Zones(k.Zones).Matches(state.Name())
	log.Debugf("Autopath match '%s' for query '%s' in zone list '%v'", zone, state.Name(), k.Zones)

	if zone == "" {
		return nil
	}

	ip := state.IP()

	pod := k.podWithIP(ip)

	search := make([]string, 0, cap(k.autoPathSearch)+5)
	if zone == "." {
		if pod != nil {
			search = append(search, pod.Namespace+".svc.")
		}
		search = append(search, "svc.")
		search = append(search, ".")
	} else {
		if pod != nil {
			search = append(search, pod.Namespace+".svc."+zone)
		}
		search = append(search, "svc."+zone)
		search = append(search, zone)
	}

	search = append(search, k.autoPathSearch...)
	search = append(search, "") // sentinel
	log.Debugf("Autopath search path is '%v'", search)

	return search
}

// podWithIP return the api.Pod for source IP ip. It returns nil if nothing can be found.
func (k *Kubernetes) podWithIP(ip string) *api.Pod {
	ps := k.APIConn.PodIndex(ip)
	if len(ps) == 0 {
		return nil
	}
	return ps[0]
}
