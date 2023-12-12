package main

import (
	"context"
	raw "github.com/linode/linodego"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/rancher/kontainer-engine/types"
	"github.com/stretchr/testify/assert"
)

func TestDriver(t *testing.T) {
	t.Parallel()

	name := generateResourceName()

	token := getLinodeToken(t)

	d := &Driver{}
	client, err := d.getServiceClient(context.TODO(), token)
	if err != nil {
		t.Fatal(err)
	}

	kubernetesVersion := getLatestK8sVersion(t, client)

	opts := types.DriverOptions{
		BoolOptions: nil,
		StringOptions: map[string]string{
			"name":               name,
			"label":              name,
			"access-token":       token,
			"region":             "us-ord",
			"kubernetes-version": kubernetesVersion,
		},
		StringSliceOptions: map[string]*types.StringSlice{
			"tags": {
				Value: []string{
					"rancher",
					"lke",
					"test",
				},
			},
			"node-pools": {
				Value: []string{
					"g6-standard-1=2",
				},
			},
		},
	}
	info, err := d.Create(context.Background(), &opts, nil)
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		err = d.Remove(context.Background(), info)
		if err != nil {
			t.Fatal(err)
		}
	}()

	info, err = d.PostCheck(context.Background(), info)
	if err != nil {
		t.Fatal(err)
	}

	validateLKEClusterProperties(t, client, info, false)

	v, err := d.GetVersion(context.Background(), info)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, kubernetesVersion, v.Version, "Kubernetes version")

	c, err := d.GetClusterSize(context.Background(), info)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, int64(2), c.Count, "Cluster size")

	info, err = d.Update(context.Background(), info, &types.DriverOptions{
		StringOptions: map[string]string{
			"name":               name,
			"label":              name,
			"access-token":       token,
			"region":             "us-ord",
			"kubernetes-version": kubernetesVersion,
		},
		StringSliceOptions: map[string]*types.StringSlice{
			"node-pools": {
				Value: []string{
					"g6-standard-1=3",
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	info, err = d.PostCheck(context.Background(), info)
	if err != nil {
		t.Fatal(err)
	}

	validateLKEClusterProperties(t, client, info, false)

	uc, err := d.GetClusterSize(context.Background(), info)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, int64(3), uc.Count, "Cluster size")
}

func TestDriver_HighAvailability(t *testing.T) {
	t.Parallel()

	name := generateResourceName()

	token := getLinodeToken(t)

	d := &Driver{}
	client, err := d.getServiceClient(context.TODO(), token)
	if err != nil {
		t.Fatal(err)
	}

	kubernetesVersion := getLatestK8sVersion(t, client)

	opts := types.DriverOptions{
		BoolOptions: map[string]bool{
			"high-availability": true,
		},
		StringOptions: map[string]string{
			"name":               name,
			"label":              name,
			"access-token":       token,
			"region":             "us-ord",
			"kubernetes-version": kubernetesVersion,
		},
		StringSliceOptions: map[string]*types.StringSlice{
			"tags": {
				Value: []string{
					"rancher",
					"lke",
					"test",
				},
			},
			"node-pools": {
				Value: []string{
					"g6-standard-1=2",
				},
			},
		},
	}
	info, err := d.Create(context.Background(), &opts, nil)
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		err = d.Remove(context.Background(), info)
		if err != nil {
			t.Fatal(err)
		}
	}()

	info, err = d.PostCheck(context.Background(), info)
	if err != nil {
		t.Fatal(err)
	}

	validateLKEClusterProperties(t, client, info, true)

	v, err := d.GetVersion(context.Background(), info)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, kubernetesVersion, v.Version, "Kubernetes version")

	c, err := d.GetClusterSize(context.Background(), info)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, int64(2), c.Count, "Cluster size")
}

func TestDriver_HighAvailabilityUpgrade(t *testing.T) {
	t.Parallel()

	name := generateResourceName()

	token := getLinodeToken(t)

	d := &Driver{}
	client, err := d.getServiceClient(context.TODO(), token)
	if err != nil {
		t.Fatal(err)
	}

	kubernetesVersion := getLatestK8sVersion(t, client)

	opts := types.DriverOptions{
		BoolOptions: nil,
		StringOptions: map[string]string{
			"name":               name,
			"label":              name,
			"access-token":       token,
			"region":             "us-ord",
			"kubernetes-version": kubernetesVersion,
		},
		StringSliceOptions: map[string]*types.StringSlice{
			"tags": {
				Value: []string{
					"rancher",
					"lke",
					"test",
				},
			},
			"node-pools": {
				Value: []string{
					"g6-standard-1=2",
				},
			},
		},
	}
	info, err := d.Create(context.Background(), &opts, nil)
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		err = d.Remove(context.Background(), info)
		if err != nil {
			t.Fatal(err)
		}
	}()

	info, err = d.PostCheck(context.Background(), info)
	if err != nil {
		t.Fatal(err)
	}

	validateLKEClusterProperties(t, client, info, false)

	info, err = d.Update(context.Background(), info, &types.DriverOptions{
		BoolOptions: map[string]bool{
			"high-availability": true,
		},
		StringOptions: map[string]string{
			"name":               name,
			"label":              name,
			"access-token":       token,
			"region":             "us-ord",
			"kubernetes-version": kubernetesVersion,
		},
		StringSliceOptions: map[string]*types.StringSlice{
			"node-pools": {
				Value: []string{
					"g6-standard-1=2",
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	info, err = d.PostCheck(context.Background(), info)
	if err != nil {
		t.Fatal(err)
	}

	// Let's make sure a null HA field doesn't try to disable HA
	info, err = d.Update(context.Background(), info, &types.DriverOptions{
		StringOptions: map[string]string{
			"name":               name,
			"label":              name,
			"access-token":       token,
			"region":             "us-ord",
			"kubernetes-version": kubernetesVersion,
		},
		StringSliceOptions: map[string]*types.StringSlice{
			"node-pools": {
				Value: []string{
					"g6-standard-1=3",
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	validateLKEClusterProperties(t, client, info, true)
}

func validateLKEClusterProperties(t *testing.T, client *raw.Client, info *types.ClusterInfo, ha bool) {
	clusterID, err := strconv.Atoi(info.Metadata["cluster-id"])
	if err != nil {
		t.Fatal(err)
	}

	linodeCluster, err := client.GetLKECluster(context.Background(), clusterID)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, linodeCluster.ControlPlane.HighAvailability, ha)
}

func getLatestK8sVersion(t *testing.T, client *raw.Client) string {
	lkeVersions, err := client.ListLKEVersions(context.TODO(), &raw.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}

	lkeVersionsStr := make([]string, len(lkeVersions))
	for i, v := range lkeVersions {
		lkeVersionsStr[i] = v.ID
	}

	sort.Strings(lkeVersionsStr)

	// We should be testing on the latest version
	return lkeVersionsStr[len(lkeVersionsStr)-1]
}

func getLinodeToken(t *testing.T) string {
	token := os.Getenv("LINODE_TOKEN")
	if token == "" {
		t.Fatal("missing Linode token")
	}

	return token
}

func generateResourceName() string {
	return strings.Replace(uuid.New().String(), "-", "", -1)
}
