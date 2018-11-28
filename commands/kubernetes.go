/*
Copyright 2018 The Doctl Authors All rights reserved.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
	http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package commands

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/digitalocean/doctl"
	"github.com/digitalocean/doctl/commands/displayers"
	"github.com/digitalocean/doctl/do"
	"github.com/digitalocean/godo"
	"github.com/pborman/uuid"
	"github.com/spf13/cobra"

	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

const maxAPIFailures = 3

func errNoClusterByName(name string) error {
	return fmt.Errorf("no cluster goes by the name %q", name)
}

func errAmbigousClusterName(name string, ids []string) error {
	return fmt.Errorf("many clusters go by the name %q, they have the following IDs: %v", name, ids)
}

func errNoPoolByName(name string) error {
	return fmt.Errorf("no node pool goes by the name %q", name)
}

func errAmbigousPoolName(name string, ids []string) error {
	return fmt.Errorf("many node pools go by the name %q, they have the following IDs: %v", name, ids)
}

func errNoClusterNodeByName(name string) error {
	return fmt.Errorf("no node goes by the name %q", name)
}

func errAmbigousClusterNodeName(name string, ids []string) error {
	return fmt.Errorf("many nodes go by the name %q, they have the following IDs: %v", name, ids)
}

// Kubernetes creates the kubernetes command.
func Kubernetes() *Command {
	cmd := &Command{
		Command: &cobra.Command{
			Use:     "kubernetes",
			Aliases: []string{"kube", "k8s", "k"},
			Short:   "[beta] kubernetes commands",
			Long:    "[beta] kubernetes is used to access Kubernetes commands",
			Hidden:  !isBeta(),
		},
	}

	CmdBuilder(cmd, RunKubernetesGet, "get <id|name>", "get a cluster", Writer, aliasOpt("g"))

	CmdBuilder(cmd, RunKubernetesList, "list", "get a list of your clusters", Writer, aliasOpt("ls"))

	cmdKubeClusterCreate := CmdBuilder(cmd, RunKubernetesCreate, "create", "create a cluster", Writer, aliasOpt("c"))
	AddStringFlag(cmdKubeClusterCreate, doctl.ArgClusterName, "", "", "cluster name", requiredOpt())
	AddStringFlag(cmdKubeClusterCreate, doctl.ArgRegionSlug, "", "", "cluster region location, example value: nyc1", requiredOpt())
	AddStringFlag(cmdKubeClusterCreate, doctl.ArgClusterVersionSlug, "", "", "cluster version", requiredOpt())
	AddStringSliceFlag(cmdKubeClusterCreate, doctl.ArgTagNames, "", nil, "cluster tags")
	AddStringSliceFlag(cmdKubeClusterCreate, doctl.ArgClusterNodePools, "", nil, `cluster node pools in the form "name=your-name;size=droplet_size;count=5;tag=tag1;tag=tag2"`, requiredOpt())
	AddBoolFlag(cmdKubeClusterCreate, doctl.ArgClusterUpdateKubeconfig, "", true, "whether to add the created cluster to your kubeconfig")
	AddBoolFlag(cmdKubeClusterCreate, doctl.ArgCommandWait, "", false, "whether to wait for the created cluster become running")

	cmdKubeClusterUpdate := CmdBuilder(cmd, RunKubernetesUpdate, "update <id|name>", "update a cluster's properties", Writer, aliasOpt("u"))
	AddStringFlag(cmdKubeClusterUpdate, doctl.ArgClusterName, "", "", "cluster name")
	AddStringSliceFlag(cmdKubeClusterUpdate, doctl.ArgTagNames, "", nil, "cluster tags")
	AddBoolFlag(cmdKubeClusterUpdate, doctl.ArgClusterUpdateKubeconfig, "", true, "whether to update the cluster in your kubeconfig")

	cmdKubeClusterDelete := CmdBuilder(cmd, RunKubernetesDelete, "delete <id|name>", "delete a cluster", Writer, aliasOpt("d", "rm"))
	AddBoolFlag(cmdKubeClusterDelete, doctl.ArgForce, doctl.ArgShortForce, false, "Force cluster delete")
	AddBoolFlag(cmdKubeClusterDelete, doctl.ArgClusterUpdateKubeconfig, "", true, "whether to remove the deleted cluster to your kubeconfig")

	cmd.AddCommand(kubernetesKubeconfig())

	cmd.AddCommand(kubernetesNodePools())

	cmd.AddCommand(kubernetesOptions())

	return cmd
}

func kubernetesKubeconfig() *Command {
	cmd := &Command{
		Command: &cobra.Command{
			Use:     "kubeconfig",
			Aliases: []string{"kubecfg", "k8scfg", "config", "cfg"},
			Short:   "kubeconfig commands",
			Long:    "kubeconfig commands are used retrieve a cluster's credentials and manipulate them",
		},
	}

	CmdBuilder(cmd, RunKubernetesKubeconfigPrint, "print <cluster-id|cluster-name>", "print a cluster's kubeconfig to standard out", Writer, aliasOpt("p", "g"))
	CmdBuilder(cmd, RunKubernetesKubeconfigSave, "save <cluster-id|cluster-name>", "save a cluster's credentials to your local kubeconfig", Writer, aliasOpt("s"))
	CmdBuilder(cmd, RunKubernetesKubeconfigRemove, "remove <cluster-id|cluster-name>", "remove a cluster's credentials from your local kubeconfig", Writer, aliasOpt("d", "rm"))
	return cmd
}

func kubernetesNodePools() *Command {
	cmd := &Command{
		Command: &cobra.Command{
			Use:     "node-pool",
			Aliases: []string{"pool", "np", "p"},
			Short:   "node pool commands",
			Long:    "node pool commands are used to act on a cluster's node pools",
		},
	}

	CmdBuilder(cmd, RunClusterNodePoolGet, "get <cluster-id|cluster-name> <pool-id|pool-name>", "get a cluster's node pool", Writer, aliasOpt("g"))
	CmdBuilder(cmd, RunClusterNodePoolList, "list <cluster-id|cluster-name>", "list a cluster's node pools", Writer, aliasOpt("ls"))

	cmdKubeNodePoolCreate := CmdBuilder(cmd, RunClusterNodePoolCreate, "create <cluster-id|cluster-name>", "create a new node pool for a cluster", Writer, aliasOpt("c"))
	AddStringFlag(cmdKubeNodePoolCreate, doctl.ArgNodePoolName, "", "", "node pool name", requiredOpt())
	AddStringFlag(cmdKubeNodePoolCreate, doctl.ArgSizeSlug, "", "", "size of nodes in the node pool", requiredOpt())
	AddStringFlag(cmdKubeNodePoolCreate, doctl.ArgNodePoolCount, "", "", "count of nodes in the node pool", requiredOpt())
	AddStringFlag(cmdKubeNodePoolCreate, doctl.ArgTagNames, "", "", "tags to apply to the node pool")

	cmdKubeNodePoolUpdate := CmdBuilder(cmd, RunClusterNodePoolUpdate, "update <cluster-id|cluster-name> <pool-id|pool-name>", "update an existing node pool in a cluster", Writer, aliasOpt("u"))
	AddStringFlag(cmdKubeNodePoolUpdate, doctl.ArgNodePoolName, "", "", "node pool name")
	AddStringFlag(cmdKubeNodePoolUpdate, doctl.ArgNodePoolCount, "", "", "count of nodes in the node pool")
	AddStringFlag(cmdKubeNodePoolUpdate, doctl.ArgTagNames, "", "", "tags to apply to the node pool")

	cmdKubeNodePoolRecycle := CmdBuilder(cmd, RunClusterNodePoolRecycle, "recycle <cluster-id|cluster-name> <pool-id|pool-name>", "recycle nodes in a node pool", Writer, aliasOpt("r"))
	AddStringFlag(cmdKubeNodePoolRecycle, doctl.ArgNodePoolNodeIDs, "", "", "ID or name of the nodes in the node pool to recycle")

	cmdKubeNodePoolDelete := CmdBuilder(cmd, RunClusterNodePoolDelete, "delete <cluster-id|cluster-name> <pool-id|pool-name>", "delete node pool from a cluster", Writer, aliasOpt("d", "rm"))
	AddBoolFlag(cmdKubeNodePoolDelete, doctl.ArgForce, doctl.ArgShortForce, false, "Force node pool delete")
	return cmd
}

func kubernetesOptions() *Command {
	cmd := &Command{
		Command: &cobra.Command{
			Use:     "options",
			Aliases: []string{"opts", "o"},
			Short:   "options commands",
			Long:    "options commands are used to find options for Kubernetes clusters",
		},
	}

	CmdBuilder(cmd, RunKubeOptionsListVersion, "versions", "versions that can be used to create a Kubernetes cluster", Writer, aliasOpt("v"))
	return cmd
}

// Clusters

// RunKubernetesGet retrieves an existing kubernetes by its identifier.
func RunKubernetesGet(c *CmdConfig) error {
	if len(c.Args) != 1 {
		return doctl.NewMissingArgsErr(c.NS)
	}
	clusterIDorName := c.Args[0]

	cluster, err := clusterByIDorName(c.Kubernetes(), clusterIDorName)
	if err != nil {
		return err
	}
	return displayClusters(c, *cluster)
}

// RunKubernetesList lists kubernetess.
func RunKubernetesList(c *CmdConfig) error {
	kube := c.Kubernetes()
	list, err := kube.List()
	if err != nil {
		return err
	}

	return displayClusters(c, list...)
}

// RunKubernetesCreate creates a new kubernetes with a given configuration.
func RunKubernetesCreate(c *CmdConfig) error {
	r := new(godo.KubernetesClusterCreateRequest)
	if err := buildClusterCreateRequestFromArgs(c, r); err != nil {
		return err
	}
	wait, err := c.Doit.GetBool(c.NS, doctl.ArgCommandWait)
	if err != nil {
		return err
	}
	update, err := c.Doit.GetBool(c.NS, doctl.ArgClusterUpdateKubeconfig)
	if err != nil {
		return err
	}

	kube := c.Kubernetes()

	cluster, err := kube.Create(r)
	if err != nil {
		return err
	}

	if update {
		tryUpdateKubeconfig(kube, cluster.ID)
	}

	if wait {
		err = waitForClusterRunning(kube, cluster.ID)
		if err != nil {
			warn(fmt.Sprintf("cluster didn't become running: %v", err))
		}
	}

	return displayClusters(c, *cluster)
}

// RunKubernetesUpdate updates an existing kubernetes with new configuration.
func RunKubernetesUpdate(c *CmdConfig) error {
	if len(c.Args) == 0 {
		return doctl.NewMissingArgsErr(c.NS)
	}
	update, err := c.Doit.GetBool(c.NS, doctl.ArgClusterUpdateKubeconfig)
	if err != nil {
		return err
	}
	clusterID, err := clusterIDize(c.Kubernetes(), c.Args[0])
	if err != nil {
		return err
	}

	r := new(godo.KubernetesClusterUpdateRequest)
	if err := buildClusterUpdateRequestFromArgs(c, r); err != nil {
		return err
	}

	kube := c.Kubernetes()
	cluster, err := kube.Update(clusterID, r)
	if err != nil {
		return err
	}

	if update {
		tryUpdateKubeconfig(kube, clusterID)
	}

	return displayClusters(c, *cluster)
}

func tryUpdateKubeconfig(kube do.KubernetesService, clusterID string) {
	var (
		kubeconfig []byte
		err        error
	)
	for tries := 0; ; tries++ {
		kubeconfig, err = kube.GetKubeConfig(clusterID)
		if err == nil {
			break
		}
		if tries >= maxAPIFailures {
			warn(fmt.Sprintf("couldn't get credentials for cluster, it will not be added to your kubeconfig: %v", err))
			return
		}
		time.Sleep(2 * time.Second)
	}
	if err := writeOrAddToKubeconfig(kubeconfig); err != nil {
		warn(fmt.Sprintf("couldn't write cluster credentials: %v", err))
	}
}

// RunKubernetesDelete deletes a kubernetes by its identifier.
func RunKubernetesDelete(c *CmdConfig) error {
	if len(c.Args) != 1 {
		return doctl.NewMissingArgsErr(c.NS)
	}
	update, err := c.Doit.GetBool(c.NS, doctl.ArgClusterUpdateKubeconfig)
	if err != nil {
		return err
	}
	clusterID, err := clusterIDize(c.Kubernetes(), c.Args[0])
	if err != nil {
		return err
	}

	force, err := c.Doit.GetBool(c.NS, doctl.ArgForce)
	if err != nil {
		return err
	}

	if force || AskForConfirm("delete this Kubernetes cluster") == nil {
		// continue
	} else {
		return fmt.Errorf("operation aborted")
	}
	kube := c.Kubernetes()

	var kubeconfig []byte
	if update {
		// get the cluster's kubeconfig before issuing the delete, so that we can
		// cleanup the entry from the local file
		kubeconfig, err = kube.GetKubeConfig(clusterID)
		if err != nil {
			warn("couldn't get credentials for cluster, it will not be remove from your kubeconfig")
		}
	}
	if err := kube.Delete(clusterID); err != nil {
		return err
	}
	if kubeconfig != nil {
		if err := removeFromKubeconfig(kubeconfig); err != nil {
			warn("Cluster was deleted, but we couldn't remove it from your local kubeconfig. Try doing it manually.")
		}
	}

	return nil
}

// Kubeconfig

// RunKubernetesKubeconfigPrint retrieves an existing kubernetes config and prints it.
func RunKubernetesKubeconfigPrint(c *CmdConfig) error {
	if len(c.Args) != 1 {
		return doctl.NewMissingArgsErr(c.NS)
	}
	kube := c.Kubernetes()
	clusterID, err := clusterIDize(kube, c.Args[0])
	if err != nil {
		return err
	}
	kubeconfig, err := kube.GetKubeConfig(clusterID)
	if err != nil {
		return err
	}
	_, err = c.Out.Write(kubeconfig)
	return err
}

// RunKubernetesKubeconfigSave retrieves an existing kubernetes config and saves it to your local kubeconfig.
func RunKubernetesKubeconfigSave(c *CmdConfig) error {
	if len(c.Args) != 1 {
		return doctl.NewMissingArgsErr(c.NS)
	}
	kube := c.Kubernetes()
	clusterID, err := clusterIDize(kube, c.Args[0])
	if err != nil {
		return err
	}
	kubeconfig, err := kube.GetKubeConfig(clusterID)
	if err != nil {
		return err
	}
	if err := writeOrAddToKubeconfig(kubeconfig); err != nil {
		return err
	}
	return nil
}

// RunKubernetesKubeconfigRemove retrieves an existing kubernetes config and removes it from your local kubeconfig.
func RunKubernetesKubeconfigRemove(c *CmdConfig) error {
	if len(c.Args) != 1 {
		return doctl.NewMissingArgsErr(c.NS)
	}
	kube := c.Kubernetes()
	clusterID, err := clusterIDize(kube, c.Args[0])
	if err != nil {
		return err
	}
	kubeconfig, err := kube.GetKubeConfig(clusterID)
	if err != nil {
		return err
	}
	if err := removeFromKubeconfig(kubeconfig); err != nil {
		return err
	}
	return nil
}

// Node Pools

// RunClusterNodePoolGet retrieves an existing cluster node pool by its identifier.
func RunClusterNodePoolGet(c *CmdConfig) error {
	if len(c.Args) != 2 {
		return doctl.NewMissingArgsErr(c.NS)
	}
	clusterID, err := clusterIDize(c.Kubernetes(), c.Args[0])
	if err != nil {
		return err
	}
	nodePool, err := poolByIDorName(c.Kubernetes(), clusterID, c.Args[1])
	if err != nil {
		return err
	}
	return displayNodePools(c, *nodePool)
}

// RunClusterNodePoolList lists cluster node pool.
func RunClusterNodePoolList(c *CmdConfig) error {
	if len(c.Args) != 1 {
		return doctl.NewMissingArgsErr(c.NS)
	}
	clusterID, err := clusterIDize(c.Kubernetes(), c.Args[0])
	if err != nil {
		return err
	}
	kube := c.Kubernetes()
	list, err := kube.ListNodePools(clusterID)
	if err != nil {
		return err
	}

	return displayNodePools(c, list...)
}

// RunClusterNodePoolCreate creates a new cluster node pool with a given configuration.
func RunClusterNodePoolCreate(c *CmdConfig) error {
	if len(c.Args) != 1 {
		return doctl.NewMissingArgsErr(c.NS)
	}
	clusterID, err := clusterIDize(c.Kubernetes(), c.Args[0])
	if err != nil {
		return err
	}

	r := new(godo.KubernetesNodePoolCreateRequest)
	if err := buildNodePoolCreateRequestFromArgs(c, r); err != nil {
		return err
	}

	kube := c.Kubernetes()
	nodePool, err := kube.CreateNodePool(clusterID, r)
	if err != nil {
		return err
	}

	return displayNodePools(c, *nodePool)
}

// RunClusterNodePoolUpdate updates an existing cluster node pool with new properties.
func RunClusterNodePoolUpdate(c *CmdConfig) error {
	if len(c.Args) != 2 {
		return doctl.NewMissingArgsErr(c.NS)
	}
	clusterID, err := clusterIDize(c.Kubernetes(), c.Args[0])
	if err != nil {
		return err
	}
	poolID, err := poolIDize(c.Kubernetes(), clusterID, c.Args[1])
	if err != nil {
		return err
	}

	r := new(godo.KubernetesNodePoolUpdateRequest)
	if err := buildNodePoolUpdateRequestFromArgs(c, r); err != nil {
		return err
	}

	kube := c.Kubernetes()
	nodePool, err := kube.UpdateNodePool(clusterID, poolID, r)
	if err != nil {
		return err
	}

	return displayNodePools(c, *nodePool)
}

// RunClusterNodePoolRecycle recycles an existing kubernetes with new configuration.
func RunClusterNodePoolRecycle(c *CmdConfig) error {
	if len(c.Args) != 2 {
		return doctl.NewMissingArgsErr(c.NS)
	}
	clusterID, err := clusterIDize(c.Kubernetes(), c.Args[0])
	if err != nil {
		return err
	}
	poolID, err := poolIDize(c.Kubernetes(), clusterID, c.Args[1])
	if err != nil {
		return err
	}

	r := new(godo.KubernetesNodePoolRecycleNodesRequest)
	if err := buildNodePoolRecycleRequestFromArgs(c, clusterID, poolID, r); err != nil {
		return err
	}

	kube := c.Kubernetes()
	return kube.RecycleNodePoolNodes(clusterID, poolID, r)
}

// RunClusterNodePoolDelete deletes a kubernetes by its identifier.
func RunClusterNodePoolDelete(c *CmdConfig) error {
	if len(c.Args) != 2 {
		return doctl.NewMissingArgsErr(c.NS)
	}
	clusterID, err := clusterIDize(c.Kubernetes(), c.Args[0])
	if err != nil {
		return err
	}
	poolID, err := poolIDize(c.Kubernetes(), clusterID, c.Args[1])
	if err != nil {
		return err
	}

	force, err := c.Doit.GetBool(c.NS, doctl.ArgForce)
	if err != nil {
		return err
	}
	if force || AskForConfirm("delete this Kubernetes node pool") == nil {
		kube := c.Kubernetes()
		if err := kube.DeleteNodePool(clusterID, poolID); err != nil {
			return err
		}
	} else {
		return fmt.Errorf("operation aborted")
	}
	return nil
}

// RunKubeOptionsListVersion deletes a kubernetes by its identifier.
func RunKubeOptionsListVersion(c *CmdConfig) error {
	kube := c.Kubernetes()
	versions, err := kube.GetVersions()
	if err != nil {
		return err
	}
	item := &displayers.KubernetesVersions{KubernetesVersions: versions}
	return c.Display(item)
}

func buildClusterCreateRequestFromArgs(c *CmdConfig, r *godo.KubernetesClusterCreateRequest) error {
	name, err := c.Doit.GetString(c.NS, doctl.ArgClusterName)
	if err != nil {
		return err
	}
	r.Name = name

	region, err := c.Doit.GetString(c.NS, doctl.ArgRegionSlug)
	if err != nil {
		return err
	}
	r.RegionSlug = region

	version, err := c.Doit.GetString(c.NS, doctl.ArgClusterVersionSlug)
	if err != nil {
		return err
	}
	r.VersionSlug = version

	tags, err := c.Doit.GetStringSlice(c.NS, doctl.ArgTagNames)
	if err != nil {
		return err
	}
	r.Tags = tags

	nodePools, err := buildNodePoolCreateRequestsFromArgs(c)
	if err != nil {
		return err
	}
	r.NodePools = nodePools

	return nil
}

func buildClusterUpdateRequestFromArgs(c *CmdConfig, r *godo.KubernetesClusterUpdateRequest) error {
	name, err := c.Doit.GetString(c.NS, doctl.ArgClusterName)
	if err != nil {
		return err
	}
	r.Name = name

	tags, err := c.Doit.GetStringSlice(c.NS, doctl.ArgTagNames)
	if err != nil {
		return err
	}
	r.Tags = tags

	return nil
}

func buildNodePoolRecycleRequestFromArgs(c *CmdConfig, clusterID, poolID string, r *godo.KubernetesNodePoolRecycleNodesRequest) error {
	nodeIDorNames, err := c.Doit.GetStringSlice(c.NS, doctl.ArgNodePoolNodeIDs)
	if err != nil {
		return err
	}
	allUUIDs := true
	for _, node := range nodeIDorNames {
		if !looksLikeUUID(node) {
			allUUIDs = false
		}
	}
	if allUUIDs {
		r.Nodes = nodeIDorNames
	} else {
		// at least some of the args weren't UUIDs, so assume that they're all names
		nodes, err := nodesByNames(c.Kubernetes(), clusterID, poolID, nodeIDorNames)
		if err != nil {
			return err
		}
		for _, node := range nodes {
			r.Nodes = append(r.Nodes, node.ID)
		}
	}
	return nil
}

func buildNodePoolCreateRequestsFromArgs(c *CmdConfig) ([]*godo.KubernetesNodePoolCreateRequest, error) {
	nodePools, err := c.Doit.GetStringSlice(c.NS, doctl.ArgClusterNodePools)
	if err != nil {
		return nil, err
	}
	out := make([]*godo.KubernetesNodePoolCreateRequest, 0, len(nodePools))
	for i, nodePoolString := range nodePools {
		poolCreateReq, err := parseNodePoolString(nodePoolString)
		if err != nil {
			return nil, fmt.Errorf("invalid node pool arguments for flag %d: %v", i, err)
		}
		out = append(out, poolCreateReq)
	}
	return out, nil
}

func parseNodePoolString(nodePool string) (*godo.KubernetesNodePoolCreateRequest, error) {
	const (
		argSeparator = ";"
		kvSeparator  = "="
	)
	out := new(godo.KubernetesNodePoolCreateRequest)
	for _, arg := range strings.Split(nodePool, argSeparator) {
		kvs := strings.SplitN(arg, kvSeparator, 2)
		if len(kvs) < 2 {
			return nil, fmt.Errorf("a node pool string argument must be of the form `key=value`, got KVs %v", kvs)
		}
		key := kvs[0]
		value := kvs[1]
		switch key {
		case "name":
			out.Name = value
		case "size":
			out.Size = value
		case "count":
			count, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return nil, errors.New("node pool count argument must be a valid integer")
			}
			out.Count = int(count)
		case "tag":
			out.Tags = append(out.Tags, value)
		default:
			return nil, fmt.Errorf("unsupported node pool argument %q", key)
		}
	}
	return out, nil
}

func buildNodePoolCreateRequestFromArgs(c *CmdConfig, r *godo.KubernetesNodePoolCreateRequest) error {
	name, err := c.Doit.GetString(c.NS, doctl.ArgNodePoolName)
	if err != nil {
		return err
	}
	r.Name = name

	size, err := c.Doit.GetString(c.NS, doctl.ArgSizeSlug)
	if err != nil {
		return err
	}
	r.Size = size

	count, err := c.Doit.GetInt(c.NS, doctl.ArgNodePoolCount)
	if err != nil {
		return err
	}
	r.Count = count

	tags, err := c.Doit.GetStringSlice(c.NS, doctl.ArgTagNames)
	if err != nil {
		return err
	}
	r.Tags = tags

	return nil
}

func buildNodePoolUpdateRequestFromArgs(c *CmdConfig, r *godo.KubernetesNodePoolUpdateRequest) error {
	name, err := c.Doit.GetString(c.NS, doctl.ArgNodePoolName)
	if err != nil {
		return err
	}
	r.Name = name

	count, err := c.Doit.GetInt(c.NS, doctl.ArgNodePoolCount)
	if err != nil {
		return err
	}
	r.Count = count

	tags, err := c.Doit.GetStringSlice(c.NS, doctl.ArgTagNames)
	if err != nil {
		return err
	}
	r.Tags = tags

	return nil
}

func writeOrAddToKubeconfig(kubeconfig []byte) error {
	remote, err := clientcmd.Load(kubeconfig)
	if err != nil {
		return err
	}
	kubectlDefaults := clientcmd.NewDefaultPathOptions()
	currentConfig, err := kubectlDefaults.GetStartingConfig()
	if err != nil {
		return err
	}
	if err := mergeKubeconfig(remote, currentConfig); err != nil {
		return fmt.Errorf("couldn't use the kubeconfig info received, %v", err)
	}
	currentConfig.CurrentContext = remote.CurrentContext
	return clientcmd.ModifyConfig(kubectlDefaults, *currentConfig, false)
}

func removeFromKubeconfig(kubeconfig []byte) error {
	remote, err := clientcmd.Load(kubeconfig)
	if err != nil {
		return err
	}
	kubectlDefaults := clientcmd.NewDefaultPathOptions()
	currentConfig, err := kubectlDefaults.GetStartingConfig()
	if err != nil {
		return err
	}
	if err := removeKubeconfig(remote, currentConfig); err != nil {
		return fmt.Errorf("couldn't use the kubeconfig info received, %v", err)
	}
	return clientcmd.ModifyConfig(kubectlDefaults, *currentConfig, false)
}

// mergeKubeconfig merges a remote cluster's config file with a local config file,
// assuming that the current context in the remote config file points to the
// cluster details to add to the local config.
func mergeKubeconfig(remote, local *clientcmdapi.Config) error {
	remoteCtx, ok := remote.Contexts[remote.CurrentContext]
	if !ok {
		// this is a bug in the backend, we received incomplete/non-sensical data
		return fmt.Errorf("the remote config has no context entry named %q. This is a bug, please open a ticket with DigitalOcean!",
			remote.CurrentContext,
		)
	}
	remoteCluster, ok := remote.Clusters[remoteCtx.Cluster]
	if !ok {
		// this is a bug in the backend, we received incomplete/non-sensical data
		return fmt.Errorf("the remote config has no cluster entry named %q. This is a bug, please open a ticket with DigitalOcean!",
			remoteCtx.Cluster,
		)
	}
	remoteAuthInfo, ok := remote.AuthInfos[remoteCtx.AuthInfo]
	if !ok {
		// this is a bug in the backend, we received incomplete/non-sensical data
		return fmt.Errorf("the remote config has no user entry named %q. This is a bug, please open a ticket with DigitalOcean!",
			remoteCtx.AuthInfo,
		)
	}

	local.Contexts[remote.CurrentContext] = remoteCtx
	local.Clusters[remoteCtx.Cluster] = remoteCluster
	local.AuthInfos[remoteCtx.AuthInfo] = remoteAuthInfo
	return nil
}

// removeKubeconfig removes a remote cluster's config file from a local config file,
// assuming that the current context in the remote config file points to the
// cluster details to reomve from the local config.
func removeKubeconfig(remote, local *clientcmdapi.Config) error {
	remoteCtx, ok := remote.Contexts[remote.CurrentContext]
	if !ok {
		// this is a bug in the backend, we received incomplete/non-sensical data
		return fmt.Errorf("the remote config has no context entry named %q. This is a bug, please open a ticket with DigitalOcean!",
			remote.CurrentContext,
		)
	}

	delete(local.Contexts, remote.CurrentContext)
	delete(local.Clusters, remoteCtx.Cluster)
	delete(local.AuthInfos, remoteCtx.AuthInfo)
	if local.CurrentContext == remote.CurrentContext {
		local.CurrentContext = ""
	}
	return nil
}

// waitForClusterRunning waits for a cluster to be running.
func waitForClusterRunning(kube do.KubernetesService, clusterID string) error {
	failCount := 0
	for {
		cluster, err := kube.Get(clusterID)
		if err != nil {
			if failCount >= maxAPIFailures {
				return err
			}
			// tolerate transient API failures
			time.Sleep(time.Second)
		} else {
			failCount = 0 // API responded, reset it's error counter
		}
		if cluster.Status == nil {
			time.Sleep(1 * time.Second)
			continue
		}
		switch cluster.Status.State {
		case godo.KubernetesClusterStatusRunning:
			return nil
		case godo.KubernetesClusterStatusProvisioning:
			time.Sleep(5 * time.Second)
		default:
			return fmt.Errorf("unknown status: [%s]", cluster.Status.State)
		}
	}
}

func displayClusters(c *CmdConfig, clusters ...do.KubernetesCluster) error {
	item := &displayers.KubernetesClusters{KubernetesClusters: do.KubernetesClusters(clusters)}
	return c.Display(item)
}

func displayNodePools(c *CmdConfig, nodePools ...do.KubernetesNodePool) error {
	item := &displayers.KubernetesNodePools{KubernetesNodePools: do.KubernetesNodePools(nodePools)}
	return c.Display(item)
}

// clusterByIDorName attempts to find a cluster by ID or by name if the argument isn't an ID. If multiple
// clusters have the same name, then an error with the cluster IDs matching this name is returned.
func clusterByIDorName(kube do.KubernetesService, idOrName string) (*do.KubernetesCluster, error) {
	if looksLikeUUID(idOrName) {
		clusterID := idOrName
		return kube.Get(clusterID)
	}
	clusters, err := kube.List()
	if err != nil {
		return nil, err
	}
	var out []*do.KubernetesCluster
	for _, c := range clusters {
		c1 := c
		if c.Name == idOrName {
			out = append(out, &c1)
		}
	}
	switch {
	case len(out) == 0:
		return nil, errNoClusterByName(idOrName)
	case len(out) > 1:
		var ids []string
		for _, c := range out {
			ids = append(ids, c.ID)
		}
		return nil, errAmbigousClusterName(idOrName, ids)
	default:
		if len(out) != 1 {
			panic("the default case should always have len(out) == 1")
		}
		return out[0], nil
	}
}

// clusterIDize attempts to make a cluster ID/name string be a cluster ID.
// use this as opposed to `clusterByIDorName` if you just care about getting
// a cluster ID and don't need the cluster object itself
func clusterIDize(kube do.KubernetesService, idOrName string) (string, error) {
	if looksLikeUUID(idOrName) {
		return idOrName, nil
	}
	clusters, err := kube.List()
	if err != nil {
		return "", err
	}
	var ids []string
	for _, c := range clusters {
		if c.Name == idOrName {
			id := c.ID
			ids = append(ids, id)
		}
	}
	switch {
	case len(ids) == 0:
		return "", errNoClusterByName(idOrName)
	case len(ids) > 1:
		return "", errAmbigousClusterName(idOrName, ids)
	default:
		if len(ids) != 1 {
			panic("the default case should always have len(ids) == 1")
		}
		return ids[0], nil
	}
}

// poolByIDorName attempts to find a pool by ID or by name if the argument isn't an ID. If multiple
// pools have the same name, then an error with the pool IDs matching this name is returned.
func poolByIDorName(kube do.KubernetesService, clusterID, idOrName string) (*do.KubernetesNodePool, error) {
	if looksLikeUUID(idOrName) {
		poolID := idOrName
		return kube.GetNodePool(clusterID, poolID)
	}
	nodePools, err := kube.ListNodePools(clusterID)
	if err != nil {
		return nil, err
	}
	var out []*do.KubernetesNodePool
	for _, c := range nodePools {
		c1 := c
		if c.Name == idOrName {
			out = append(out, &c1)
		}
	}
	switch {
	case len(out) == 0:
		return nil, errNoPoolByName(idOrName)
	case len(out) > 1:
		var ids []string
		for _, c := range out {
			ids = append(ids, c.ID)
		}
		return nil, errAmbigousPoolName(idOrName, ids)
	default:
		if len(out) != 1 {
			panic("the default case should always have len(out) == 1")
		}
		return out[0], nil
	}
}

// poolIDize attempts to make a node pool ID/name string be a node pool ID.
// use this as opposed to `poolByIDorName` if you just care about getting
// a node pool ID and don't need the node pool object itself
func poolIDize(kube do.KubernetesService, clusterID, idOrName string) (string, error) {
	if looksLikeUUID(idOrName) {
		return idOrName, nil
	}
	pools, err := kube.ListNodePools(clusterID)
	if err != nil {
		return "", err
	}
	var ids []string
	for _, c := range pools {
		if c.Name == idOrName {
			ids = append(ids, c.ID)
		}
	}
	switch {
	case len(ids) == 0:
		return "", errNoPoolByName(idOrName)
	case len(ids) > 1:
		return "", errAmbigousPoolName(idOrName, ids)
	default:
		if len(ids) != 1 {
			panic("the default case should always have len(ids) == 1")
		}
		return ids[0], nil
	}
}

// nodesByNames attempts to find nodes by names. If multiple nodes have the same name,
// then an error with the node IDs matching this name is returned.
func nodesByNames(kube do.KubernetesService, clusterID, poolID string, nodeNames []string) ([]*godo.KubernetesNode, error) {
	nodePool, err := kube.GetNodePool(clusterID, poolID)
	if err != nil {
		return nil, err
	}
	var out []*godo.KubernetesNode
	for _, name := range nodeNames {
		node, err := nodeByName(name, nodePool.Nodes)
		if err != nil {
			return nil, err
		}
		out = append(out, node)
	}
	return out, nil
}

func nodeByName(name string, nodes []*godo.KubernetesNode) (*godo.KubernetesNode, error) {
	var out []*godo.KubernetesNode
	for _, n := range nodes {
		n1 := n
		if n.Name == name {
			out = append(out, n1)
		}
	}
	switch {
	case len(out) == 0:
		return nil, errNoClusterNodeByName(name)
	case len(out) > 1:
		var ids []string
		for _, c := range out {
			ids = append(ids, c.ID)
		}
		return nil, errAmbigousClusterNodeName(name, ids)
	default:
		if len(out) != 1 {
			panic("the default case should always have len(out) == 1")
		}
		return out[0], nil
	}
}

func looksLikeUUID(str string) bool {
	return uuid.Parse(str) != nil
}
