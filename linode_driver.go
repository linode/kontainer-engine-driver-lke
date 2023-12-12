package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	raw "github.com/linode/linodego"
	"github.com/linode/linodego/k8s"
	k8scondition "github.com/linode/linodego/k8s/pkg/condition"
	"github.com/rancher/kontainer-engine/drivers/options"
	"github.com/rancher/kontainer-engine/types"
	"github.com/sirupsen/logrus"
	"golang.org/x/oauth2"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/clientcmd"
)

// DefaultLinodeURL is the Linode APIv4 URL to use
const (
	DefaultLinodeURL           = "https://api.linode.com"
	retryInterval              = 5 * time.Second
	serviceAccountRetryTimeout = 5 * time.Minute
)

// Driver defines the struct of lke driver
type Driver struct {
	driverCapabilities types.Capabilities
}

type state struct {
	AccessToken string

	// The name of this cluster
	Name  string
	Label string
	// An optional description of this cluster
	Description string

	// The region to launch the cluster
	Region string
	// The kubernetes version
	K8sVersion string
	// Label      string // name ?
	Tags      []string
	NodePools map[string]int // type -> count

	// Whether this is an HA cluster (nullable)
	HighAvailability *bool

	// cluster info
	ClusterInfo types.ClusterInfo
}

func NewDriver() types.Driver {
	driver := &Driver{
		driverCapabilities: types.Capabilities{
			Capabilities: make(map[int64]bool),
		},
	}

	driver.driverCapabilities.AddCapability(types.GetVersionCapability)
	driver.driverCapabilities.AddCapability(types.SetVersionCapability)
	driver.driverCapabilities.AddCapability(types.GetClusterSizeCapability)
	driver.driverCapabilities.AddCapability(types.SetClusterSizeCapability)

	return driver
}

// GetDriverCreateOptions implements driver interface
func (d *Driver) GetDriverCreateOptions(ctx context.Context) (*types.DriverFlags, error) {
	driverFlag := types.DriverFlags{
		Options: make(map[string]*types.Flag),
	}

	driverFlag.Options["access-token"] = &types.Flag{
		Type:  types.StringType,
		Usage: "Linode api access token",
	}

	driverFlag.Options["name"] = &types.Flag{
		Type:  types.StringType,
		Usage: "the internal name of the cluster in Rancher",
	}
	driverFlag.Options["label"] = &types.Flag{
		Type:  types.StringType,
		Usage: "the label of the cluster in Linode",
	}
	driverFlag.Options["description"] = &types.Flag{
		Type:  types.StringType,
		Usage: "An optional description of this cluster",
	}

	driverFlag.Options["region"] = &types.Flag{
		Type:  types.StringType,
		Usage: "The region to launch the cluster",
		Default: &types.Default{
			DefaultString: "us-central1-a",
		},
	}
	driverFlag.Options["tags"] = &types.Flag{
		Type:  types.StringSliceType,
		Usage: "The map of Kubernetes labels (key/value pairs) to be applied to each node",
	}
	driverFlag.Options["kubernetes-version"] = &types.Flag{
		Type:  types.StringType,
		Usage: "The kubernetes version",
	}
	driverFlag.Options["node-pools"] = &types.Flag{
		Type:  types.StringSliceType,
		Usage: "The list of node pools created for the cluster",
	}

	driverFlag.Options["high-availability"] = &types.Flag{
		Type:  types.BoolPointerType,
		Usage: "If enabled, this cluster will be a high availability cluster",
	}

	return &driverFlag, nil
}

// GetDriverUpdateOptions implements driver interface
func (d *Driver) GetDriverUpdateOptions(ctx context.Context) (*types.DriverFlags, error) {
	driverFlag := types.DriverFlags{
		Options: make(map[string]*types.Flag),
	}

	driverFlag.Options["tags"] = &types.Flag{
		Type:  types.StringSliceType,
		Usage: "The map of Kubernetes labels (key/value pairs) to be applied to each node",
		Default: &types.Default{
			DefaultStringSlice: &types.StringSlice{
				Value: []string{},
			},
		},
	}

	driverFlag.Options["node-pools"] = &types.Flag{
		Type:  types.StringSliceType,
		Usage: "The list of node pools created for the cluster",
	}

	driverFlag.Options["high-availability"] = &types.Flag{
		Type:  types.BoolPointerType,
		Usage: "If enabled, this cluster will be a high availability cluster",
	}

	return &driverFlag, nil
}

// SetDriverOptions implements driver interface
func getStateFromOpts(driverOptions *types.DriverOptions) (state, error) {
	d := state{
		Tags:      []string{},
		NodePools: map[string]int{},
		ClusterInfo: types.ClusterInfo{
			Metadata: map[string]string{},
		},
	}

	d.Name = options.GetValueFromDriverOptions(driverOptions, types.StringType, "name").(string)
	d.Label = options.GetValueFromDriverOptions(driverOptions, types.StringType, "label").(string)
	d.Description = options.GetValueFromDriverOptions(driverOptions, types.StringType, "description").(string)

	d.AccessToken = options.GetValueFromDriverOptions(driverOptions, types.StringType, "access-token", "accessToken").(string)

	d.Region = options.GetValueFromDriverOptions(driverOptions, types.StringType, "region").(string)
	d.K8sVersion = options.GetValueFromDriverOptions(driverOptions, types.StringType, "kubernetes-version", "kubernetesVersion").(string)

	// Go can't cast a nil value to *bool, so we need to manually check here
	d.HighAvailability = nil
	if ha := options.GetValueFromDriverOptions(driverOptions, types.BoolPointerType,
		"high-availability", "highAvailability"); ha != nil {
		d.HighAvailability = ha.(*bool)
	}

	d.Tags = []string{}
	tags := options.GetValueFromDriverOptions(driverOptions, types.StringSliceType, "tags")
	if tags != nil {
		tags := tags.(*types.StringSlice)
		d.Tags = append(d.Tags, tags.Value...)
	}

	pools := options.GetValueFromDriverOptions(driverOptions, types.StringSliceType, "node-pools", "nodePools")
	if pools != nil {
		for _, part := range pools.(*types.StringSlice).Value {
			kv := strings.Split(part, "=")
			if len(kv) == 2 {
				count, err := strconv.Atoi(kv[1])
				if err != nil {
					return state{}, fmt.Errorf("failed to parse node count %v for pool of node type %s", kv[1], kv[0])
				}
				d.NodePools[kv[0]] = count
			}
		}
	}

	return d, d.validate()
}

func (s *state) validate() error {
	if len(s.NodePools) == 0 {
		return fmt.Errorf("at least one NodePool is required")
	}
	for t, count := range s.NodePools {
		if count <= 0 {
			return fmt.Errorf("at least 1 node required for NodePool=%s", t)
		}
	}
	return nil
}

// Create implements driver interface
func (d *Driver) Create(ctx context.Context, opts *types.DriverOptions, _ *types.ClusterInfo) (*types.ClusterInfo, error) {
	state, err := getStateFromOpts(opts)
	if err != nil {
		return nil, err
	}

	logrus.Debugf("state.name %s, state: %#v", state.Name, state)

	info := &types.ClusterInfo{}
	err = storeState(info, state)
	if err != nil {
		return info, err
	}

	client, err := d.getServiceClient(ctx, state.AccessToken)
	if err != nil {
		return info, err
	}

	req := d.generateClusterCreateRequest(state)
	logrus.Debugf("LKE api request: %#v", req)

	cluster, err := client.CreateLKECluster(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to create LKE cluster: %s", err)
	}
	info.Metadata["cluster-id"] = strconv.Itoa(cluster.ID)

	err = client.WaitForLKEClusterConditions(ctx, cluster.ID, raw.LKEClusterPollOptions{
		Retry:          true,
		TimeoutSeconds: 20 * 60,
	}, k8scondition.ClusterHasReadyNode)
	if err != nil {
		return nil, fmt.Errorf("failed to wait for lke cluster ready node: %s", err)
	}

	return info, err
}

func storeState(info *types.ClusterInfo, state state) error {
	bytes, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if info.Metadata == nil {
		info.Metadata = map[string]string{}
	}
	info.Metadata["state"] = string(bytes)
	info.Metadata["region"] = state.Region
	return nil
}

func getState(info *types.ClusterInfo) (state, error) {
	state := state{}
	// ignore error
	err := json.Unmarshal([]byte(info.Metadata["state"]), &state)
	return state, err
}

// Update implements driver interface
func (d *Driver) Update(ctx context.Context, info *types.ClusterInfo, opts *types.DriverOptions) (*types.ClusterInfo, error) {
	state, err := getState(info)
	if err != nil {
		return nil, err
	}

	logrus.Debugf("state.name %s, state: %#v", state.Name, state)

	newState, err := getStateFromOpts(opts)
	if err != nil {
		return nil, err
	}

	state.AccessToken = newState.AccessToken

	client, err := d.getServiceClient(ctx, state.AccessToken)
	if err != nil {
		return nil, err
	}

	clusterID, err := strconv.Atoi(info.Metadata["cluster-id"])
	if err != nil {
		return nil, fmt.Errorf("failed to parse cluster id: %s", err)
	}

	stateHAOk := state.HighAvailability != nil
	newStateHAOk := newState.HighAvailability != nil

	updateOpts := raw.LKEClusterUpdateOptions{}
	shouldUpdate := false

	if state.Label != newState.Label {
		updateOpts.Label = newState.Label
		shouldUpdate = true
	}

	if !sets.NewString(state.Tags...).Equal(sets.NewString(newState.Tags...)) {
		updateOpts.Tags = &newState.Tags
		state.Tags = newState.Tags
		shouldUpdate = true
	}

	// We should only update HA under certain conditions
	if newStateHAOk && (!stateHAOk || *state.HighAvailability != *newState.HighAvailability) {
		updateOpts.ControlPlane = &raw.LKEClusterControlPlane{
			HighAvailability: *newState.HighAvailability,
		}
		shouldUpdate = true
	}

	if shouldUpdate {
		_, err = client.UpdateLKECluster(ctx, clusterID, updateOpts)
		if err != nil {
			return nil, fmt.Errorf("failed to update cluster %d: %s", clusterID, err)
		}
	}

	pools, err := client.ListLKENodePools(ctx, clusterID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get pools for LKE cluster %d: %s", clusterID, err)
	}

	pm := make(map[string]raw.LKENodePool) // type -> pool
	for _, pool := range pools {
		if _, found := newState.NodePools[pool.Type]; !found {
			// delete
			err = client.DeleteLKENodePool(ctx, clusterID, pool.ID)
			if err != nil {
				return nil, fmt.Errorf("failed to delete cluster %s node pool type %s", state.Name, pool.Type)
			}
		} else {
			pm[pool.Type] = pool // id, count
		}
		delete(state.NodePools, pool.Type)
	}

	for t, count := range newState.NodePools {
		if cur, ok := pm[t]; ok {
			if cur.Count != count {
				// update
				_, err = client.UpdateLKENodePool(ctx, clusterID, cur.ID, raw.LKENodePoolUpdateOptions{
					Count: count,
				})
				if err != nil {
					return nil, fmt.Errorf("failed to update cluster %s node pool type %s", state.Name, cur.Type)
				}
				state.NodePools[t] = count
			}
		} else {
			// create
			_, err := client.CreateLKENodePool(ctx, clusterID, raw.LKENodePoolCreateOptions{
				Count: count,
				Type:  t,
				// Disks: nil, // not supported?
			})
			if err != nil {
				return nil, fmt.Errorf("failed to create cluster %s node pool type %s", state.Name, cur.Type)
			}
			state.NodePools[t] = count
		}
	}

	pools, err = client.ListLKENodePools(context.Background(), clusterID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get pools for LKE cluster %d: %s", clusterID, err)
	}
	for _, pool := range pools {
		err = waitUntilPoolReady(ctx, client, clusterID, pool.ID)
		if err != nil {
			return nil, err
		}
	}

	return info, storeState(info, state)
}

func (d *Driver) generateClusterCreateRequest(state state) raw.LKEClusterCreateOptions {
	req := raw.LKEClusterCreateOptions{
		Label:      state.Label,
		Region:     state.Region,
		K8sVersion: state.K8sVersion,
		Tags:       state.Tags,
	}

	// We should only consider HA if it's defined
	if state.HighAvailability != nil {
		req.ControlPlane = &raw.LKEClusterControlPlane{
			HighAvailability: *state.HighAvailability,
		}
	}

	for t, count := range state.NodePools {
		req.NodePools = append(req.NodePools, raw.LKENodePoolCreateOptions{
			Type:  t,
			Count: count,
			// Disks: nil, // unsupported?
		})
	}
	return req
}

func exists(m map[string]string, key string) bool {
	if m == nil {
		return false
	}
	_, ok := m[key]
	return ok
}

func (d *Driver) PostCheck(ctx context.Context, info *types.ClusterInfo) (*types.ClusterInfo, error) {
	state, err := getState(info)
	if err != nil {
		return nil, err
	}

	var kubeconfig string
	if exists(info.Metadata, "KubeConfig") {
		kubeconfig = info.Metadata["KubeConfig"]
	} else {
		// Only load Kubeconfig during first run
		client, err := d.getServiceClient(ctx, state.AccessToken)
		if err != nil {
			return nil, err
		}

		clusterID, err := strconv.Atoi(info.Metadata["cluster-id"])
		if err != nil {
			return nil, fmt.Errorf("failed to parse cluster id: %s", err)
		}

		err = client.WaitForLKEClusterConditions(ctx, clusterID, raw.LKEClusterPollOptions{
			Retry:          true,
			TimeoutSeconds: 20 * 60,
		}, k8scondition.ClusterHasReadyNode)
		if err != nil {
			return nil, fmt.Errorf("failed to wait for lke cluster ready node: %s", err)
		}

		lkeKubeconfig, err := client.GetLKEClusterKubeconfig(ctx, clusterID)
		if err != nil {
			return nil, fmt.Errorf("failed to get kubeconfig for LKE cluster %d: %s", clusterID, err)
		}
		kubeconfig = lkeKubeconfig.KubeConfig
	}

	kubeConfigBytes, err := base64.StdEncoding.DecodeString(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to decode kubeconfig: %s", err)
	}

	cfg, err := clientcmd.RESTConfigFromKubeConfig(kubeConfigBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse LKE cluster kubeconfig: %s", err)
	}

	info.Version = state.K8sVersion
	count := 0
	for _, poolSize := range state.NodePools {
		count += poolSize
	}
	info.NodeCount = int64(count)

	info.Endpoint = cfg.Host
	info.Username = cfg.Username
	info.Password = cfg.Password
	if len(cfg.CAData) > 0 {
		info.RootCaCertificate = base64.StdEncoding.EncodeToString(cfg.CAData)
	}
	if len(cfg.CertData) > 0 {
		info.ClientCertificate = base64.StdEncoding.EncodeToString(cfg.CertData)
	}
	if len(cfg.KeyData) > 0 {
		info.ClientKey = base64.StdEncoding.EncodeToString(cfg.KeyData)
	}

	info.Metadata["KubeConfig"] = kubeconfig
	serviceAccountToken, err := generateServiceAccountTokenForLKE(kubeconfig)
	if err != nil {
		return nil, err
	}
	info.ServiceAccountToken = serviceAccountToken
	return info, nil
}

// Remove implements driver interface
func (d *Driver) Remove(ctx context.Context, info *types.ClusterInfo) error {
	state, err := getState(info)
	if err != nil {
		return err
	}

	client, err := d.getServiceClient(ctx, state.AccessToken)
	if err != nil {
		return err
	}

	clusterID, err := strconv.Atoi(info.Metadata["cluster-id"])
	if err != nil {
		return fmt.Errorf("failed to parse cluster id: %s", err)
	}

	logrus.Debugf("Removing cluster %v from zone %v", state.Name, state.Region)

	err = client.DeleteLKECluster(ctx, clusterID)
	if err != nil {
		return fmt.Errorf("failed to delete Linode LKE cluster %d: %s", clusterID, err)
	}
	_, err = client.WaitForLKEClusterStatus(ctx, clusterID, "not_ready", 10*60)
	if err != nil {
		if le, ok := err.(*raw.Error); ok && le.Code == http.StatusNotFound {
			return nil
		}
		return err
	}

	return nil
}

func (d *Driver) getServiceClient(ctx context.Context, token string) (*raw.Client, error) {
	tokenSource := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	oauthTransport := &oauth2.Transport{
		Source: tokenSource,
	}

	oauth2Client := &http.Client{
		Transport: oauthTransport,
	}
	client := raw.NewClient(oauth2Client)

	client.SetUserAgent("kontainer-engine-driver-lke")
	client.SetBaseURL(DefaultLinodeURL)

	return &client, nil
}

func generateServiceAccountTokenForLKE(kubeconfig string) (string, error) {
	result := ""

	clientset, err := k8s.BuildClientsetFromConfig(&raw.LKEClusterKubeconfig{
		KubeConfig: kubeconfig,
	}, nil)
	if err != nil {
		return "", err
	}

	err = wait.Poll(retryInterval, serviceAccountRetryTimeout, func() (done bool, err error) {
		token, err := generateServiceAccountToken(clientset)
		if err != nil {
			logrus.Debugf("retrying on service account generation error: %s", err)
			return false, nil
		}

		result = token
		return true, nil
	})

	return result, err
}

func (d *Driver) GetClusterSize(ctx context.Context, info *types.ClusterInfo) (*types.NodeCount, error) {
	state, err := getState(info)
	if err != nil {
		return nil, err
	}

	clusterID, err := strconv.Atoi(info.Metadata["cluster-id"])
	if err != nil {
		return nil, fmt.Errorf("failed to parse cluster id: %s", err)
	}

	client, err := d.getServiceClient(ctx, state.AccessToken)
	if err != nil {
		return nil, err
	}

	pools, err := client.ListLKENodePools(ctx, clusterID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get pools for LKE cluster %d: %s", clusterID, err)
	}

	count := 0
	for _, pool := range pools {
		count += pool.Count
	}
	return &types.NodeCount{Count: int64(count)}, nil
}

func (d *Driver) GetVersion(ctx context.Context, info *types.ClusterInfo) (*types.KubernetesVersion, error) {
	state, err := getState(info)
	if err != nil {
		return nil, err
	}

	clusterID, err := strconv.Atoi(info.Metadata["cluster-id"])
	if err != nil {
		return nil, fmt.Errorf("failed to parse cluster id: %s", err)
	}

	client, err := d.getServiceClient(ctx, state.AccessToken)
	if err != nil {
		return nil, err
	}

	cluster, err := client.GetLKECluster(ctx, clusterID)
	if err != nil {
		return nil, fmt.Errorf("failed to get LKE cluster %d: %s", clusterID, err)
	}
	return &types.KubernetesVersion{Version: cluster.K8sVersion}, nil
}

func (d *Driver) SetClusterSize(ctx context.Context, info *types.ClusterInfo, count *types.NodeCount) error {
	state, err := getState(info)
	if err != nil {
		return err
	}

	clusterID, err := strconv.Atoi(info.Metadata["cluster-id"])
	if err != nil {
		return fmt.Errorf("failed to parse cluster id: %s", err)
	}

	client, err := d.getServiceClient(ctx, state.AccessToken)
	if err != nil {
		return err
	}

	logrus.Info("updating cluster size")

	pools, err := client.ListLKENodePools(ctx, clusterID, nil)
	if err != nil {
		return fmt.Errorf("failed to get pools for LKE cluster %d: %s", clusterID, err)
	}

	poolID := pools[0].ID
	poolNodeCount := pools[0].Count

	_, err = client.UpdateLKENodePool(ctx, clusterID, poolID, raw.LKENodePoolUpdateOptions{
		Count: int(count.Count),
	})
	if err != nil {
		return fmt.Errorf(
			"failed to update LKE Cluster %d Node Pool %d: %w",
			clusterID,
			poolID,
			err,
		)
	}

	if poolNodeCount < int(count.Count) {
		err = waitUntilPoolReady(ctx, client, clusterID, poolID)
		if err != nil {
			return err
		}
	}

	logrus.Info("cluster size updated successfully")

	return nil
}

func waitUntilPoolReady(ctx context.Context, client *raw.Client, clusterID int, poolID int) error {
	return wait.PollImmediateInfinite(retryInterval, func() (done bool, err error) {
		pool, err := client.GetLKENodePool(ctx, clusterID, poolID)
		if err != nil {
			return false, err
		}
		for _, linode := range pool.Linodes {
			if linode.Status != raw.LKELinodeReady {
				return false, nil
			}
		}
		return true, nil
	})
}

func (d *Driver) SetVersion(ctx context.Context, info *types.ClusterInfo, version *types.KubernetesVersion) error {
	return nil
}

func (d *Driver) GetCapabilities(ctx context.Context) (*types.Capabilities, error) {
	return &d.driverCapabilities, nil
}

func (d *Driver) ETCDSave(ctx context.Context, clusterInfo *types.ClusterInfo, opts *types.DriverOptions, snapshotName string) error {
	return fmt.Errorf("ETCD backup operations are not implemented")
}

func (d *Driver) ETCDRestore(ctx context.Context, clusterInfo *types.ClusterInfo, opts *types.DriverOptions, snapshotName string) (*types.ClusterInfo, error) {
	return nil, fmt.Errorf("ETCD backup operations are not implemented")
}

func (d *Driver) ETCDRemoveSnapshot(ctx context.Context, clusterInfo *types.ClusterInfo, opts *types.DriverOptions, snapshotName string) error {
	return fmt.Errorf("ETCD backup operations are not implemented")
}

func (d *Driver) GetK8SCapabilities(ctx context.Context, options *types.DriverOptions) (*types.K8SCapabilities, error) {
	capabilities := &types.K8SCapabilities{
		L4LoadBalancer: &types.LoadBalancerCapabilities{
			Enabled:              true,
			Provider:             "NodeBalancer", // what are the options?
			ProtocolsSupported:   []string{"TCP", "UDP"},
			HealthCheckSupported: true,
		},
	}
	return capabilities, nil
}

func (d *Driver) RemoveLegacyServiceAccount(ctx context.Context, info *types.ClusterInfo) error {
	return nil
}
