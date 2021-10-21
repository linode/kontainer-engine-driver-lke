package main

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	raw "github.com/linode/linodego"
	"github.com/rancher/kontainer-engine/types"
	"github.com/stretchr/testify/assert"
)

func TestDriver(t *testing.T) {
	name := strings.Replace(uuid.New().String(), "-", "", -1)

	token := os.Getenv("LINODE_TOKEN")
	if token == "" {
		t.Fatal("missing Linode token")
	}

	d := &Driver{}
	client, err := d.getServiceClient(context.TODO(), token)
	if err != nil {
		t.Fatal(err)
	}

	lkeVersions, err := client.ListLKEVersions(context.TODO(), &raw.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	kubernetesVersion := lkeVersions[0].ID

	opts := types.DriverOptions{
		BoolOptions: nil,
		StringOptions: map[string]string{
			"name":               name,
			"label":              name,
			"access-token":       token,
			"region":             "us-west",
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
	info, err := d.Create(context.TODO(), &opts, nil)
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		err = d.Remove(context.TODO(), info)
		if err != nil {
			t.Fatal(err)
		}
	}()

	info, err = d.PostCheck(context.TODO(), info)
	if err != nil {
		t.Fatal(err)
	}

	v, err := d.GetVersion(context.TODO(), info)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, kubernetesVersion, v.Version, "Kubernetes version")

	c, err := d.GetClusterSize(context.TODO(), info)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, int64(2), c.Count, "Cluster size")

	info, err = d.Update(context.TODO(), info, &types.DriverOptions{
		StringOptions: map[string]string{
			"name":               name,
			"label":              name,
			"access-token":       token,
			"region":             "us-west",
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

	info, err = d.PostCheck(context.TODO(), info)
	if err != nil {
		t.Fatal(err)
	}

	uc, err := d.GetClusterSize(context.TODO(), info)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, int64(3), uc.Count, "Cluster size")
}
