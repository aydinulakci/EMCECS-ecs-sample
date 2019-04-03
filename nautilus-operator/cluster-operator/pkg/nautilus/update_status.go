package nautilus

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"reflect"
	"strings"
	"time"

	nautilusv1 "github.com/nautilus/cluster-operator/pkg/apis/nautilus/v1"
	nautilusapi "github.com/nautilus/go-api"
	"github.com/nautilus/go-api/types"
	"k8s.io/api/core/v1"
)

func (s *Deployment) updateNautilusStatus(status *nautilusv1.NautilusClusterStatus) error {
	if reflect.DeepEqual(s.stos.Status, *status) {
		return nil
	}

	// When there's a difference in node ready count, broadcast the health change event.
	if s.stos.Status.Ready != status.Ready {
		// Ready contains the node count in the format 3/3.
		ready := strings.Split(status.Ready, "/")
		if ready[0] == ready[1] {
			if s.recorder != nil {
				s.recorder.Event(s.stos, v1.EventTypeNormal, "ChangedStatus", fmt.Sprintf("%s Nautilus nodes are functional. Cluster healthy", status.Ready))
			}
		} else {
			if s.recorder != nil {
				s.recorder.Event(s.stos, v1.EventTypeWarning, "ChangedStatus", fmt.Sprintf("%s Nautilus nodes are functional", status.Ready))
			}
		}
	}

	s.stos.Status = *status
	return s.client.Update(context.Background(), s.stos)
}

// getNautilusStatus queries health of all the nodes in the join token and
// returns the cluster status.
func (s *Deployment) getNautilusStatus() (*nautilusv1.NautilusClusterStatus, error) {
	nodeIPs := strings.Split(s.stos.Spec.Join, ",")

	totalNodes := len(nodeIPs)
	readyNodes := 0

	healthStatus := make(map[string]nautilusv1.NodeHealth)
	memberStatus := new(nautilusv1.MembersStatus)

	for _, node := range nodeIPs {
		if status, err := getNodeHealth(node, 1); err == nil {
			healthStatus[node] = *status
			if isHealthy(status) {
				readyNodes++
				memberStatus.Ready = append(memberStatus.Ready, node)
			} else {
				memberStatus.Unready = append(memberStatus.Unready, node)
			}
		} else {
			log.Printf("failed to get health of node %s: %v", node, err)
		}
	}

	phase := nautilusv1.ClusterPhaseInitial
	if readyNodes == totalNodes {
		phase = nautilusv1.ClusterPhaseRunning
	}

	return &nautilusv1.NautilusClusterStatus{
		Phase:            phase,
		Nodes:            nodeIPs,
		NodeHealthStatus: healthStatus,
		Ready:            fmt.Sprintf("%d/%d", readyNodes, totalNodes),
		Members:          *memberStatus,
	}, nil
}

func isHealthy(health *nautilusv1.NodeHealth) bool {
	if health.DirectfsInitiator+health.Director+health.KV+health.KVWrite+
		health.Nats+health.Presentation+health.Rdb == strings.Repeat("alive", 7) {
		return true
	}
	return false
}

func getNodeHealth(address string, timeout int) (*nautilusv1.NodeHealth, error) {
	healthEndpointFormat := "http://%s:%s/v1/" + nautilusapi.HealthAPIPrefix

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*time.Duration(timeout))
	defer cancel()

	client := &http.Client{}

	var healthStatus types.HealthStatus
	cpURL := fmt.Sprintf(healthEndpointFormat, address, nautilusapi.DefaultPort)
	cpReq, err := http.NewRequest("GET", cpURL, nil)
	if err != nil {
		return nil, err
	}

	cpResp, err := client.Do(cpReq.WithContext(ctx))
	if err != nil {
		return nil, err
	}

	if err := json.NewDecoder(cpResp.Body).Decode(&healthStatus); err != nil {
		return nil, err
	}

	return &nautilusv1.NodeHealth{
		DirectfsInitiator: healthStatus.Submodules.DirectFSClient.Status,
		Director:          healthStatus.Submodules.Director.Status,
		KV:                healthStatus.Submodules.KV.Status,
		KVWrite:           healthStatus.Submodules.KVWrite.Status,
		Nats:              healthStatus.Submodules.NATS.Status,
		Presentation:      healthStatus.Submodules.FS.Status,
		Rdb:               healthStatus.Submodules.FSDriver.Status,
	}, nil
}