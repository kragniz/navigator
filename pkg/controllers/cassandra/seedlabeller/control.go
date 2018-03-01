package seedlabeller

import (
	"fmt"

	"github.com/golang/glog"
	appsv1beta1 "k8s.io/api/apps/v1beta1"
	"k8s.io/client-go/kubernetes"
	appslisters "k8s.io/client-go/listers/apps/v1beta1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/record"

	"github.com/jetstack/navigator/pkg/apis/navigator/v1alpha1"
	"github.com/jetstack/navigator/pkg/controllers/cassandra/service"
	"github.com/jetstack/navigator/pkg/controllers/cassandra/util"
)

type Interface interface {
	Sync(*v1alpha1.CassandraCluster) error
}

type defaultSeedLabeller struct {
	kubeClient        kubernetes.Interface
	statefulSetLister appslisters.StatefulSetLister
	pods              corelisters.PodLister
	recorder          record.EventRecorder
}

var _ Interface = &defaultSeedLabeller{}

func NewControl(
	kubeClient kubernetes.Interface,
	statefulSetLister appslisters.StatefulSetLister,
	pods corelisters.PodLister,
	recorder record.EventRecorder,
) Interface {
	return &defaultSeedLabeller{
		kubeClient:        kubeClient,
		statefulSetLister: statefulSetLister,
		pods:              pods,
		recorder:          recorder,
	}
}

func (c *defaultSeedLabeller) labelSeedNodes(
	cluster *v1alpha1.CassandraCluster,
	np *v1alpha1.CassandraClusterNodePool,
	set *appsv1beta1.StatefulSet,
) error {
	for i := int64(0); i < np.Replicas; i++ {
		pod, err := c.pods.Pods(cluster.Namespace).Get(fmt.Sprintf("%s-%d", set.Name, i))
		if err != nil {
			glog.Warningf("Couldn't get stateful set pod: %v", err)
			return nil
		}

		// default to not a seed
		desiredLabel := "false"

		// label first n as seeds
		if i < np.Seeds {
			desiredLabel = seedprovider.SeedLabelValue
		}

		labels := pod.Labels
		value := labels[seedprovider.SeedLabelKey]
		if value == desiredLabel {
			continue
		}
		if labels == nil {
			labels = map[string]string{}
		}
		labels[seedprovider.SeedLabelKey] = desiredLabel
		podCopy := pod.DeepCopy()
		podCopy.SetLabels(labels)
		_, err = c.kubeClient.CoreV1().Pods(podCopy.Namespace).Update(podCopy)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *defaultSeedLabeller) Sync(cluster *v1alpha1.CassandraCluster) error {
	for _, np := range cluster.Spec.NodePools {
		setName := util.NodePoolResourceName(cluster, &np)

		set, err := c.statefulSetLister.StatefulSets(cluster.Namespace).Get(setName)
		if err != nil {
			return err
		}
		err = c.labelSeedNodes(cluster, &np, set)
		if err != nil {
			return err
		}
	}
	return nil
}
