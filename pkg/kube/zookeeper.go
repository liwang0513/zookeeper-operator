package kube

import (
	"github.com/liwang-pivotal/zookeeper-operator/spec"

	"k8s.io/api/core/v1"
	appsv1Beta2 "k8s.io/api/apps/v1beta2"
	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"time"
)

const (
	defaultCPU    = "10m"
	defaultDisk   = "1Gi"
	defaultMemory = "50Mi"
)


func CreateCluster(cluster spec.ZookeeperCluster, client Kubernetes) error {
	sts := generateZookeeperStatefulset(cluster)
	//Create Broker Cluster
	err := client.CreateOrUpdateStatefulSet(sts)
	if err != nil {
		return err
	}

	return nil
}

func DeleteCluster(cluster spec.ZookeeperCluster, client Kubernetes) error {
	cluster.Spec.BrokerCount = 0
	sts := generateZookeeperStatefulset(cluster)
	//Downsize Statefulset to 0
	err := client.CreateOrUpdateStatefulSet(sts)
	if err != nil {
		return err
	}

	//Force Delete of Statefulset
	err = client.DeleteStatefulset(sts)
	if err != nil {
		return err
	}

	return nil
}

func generateZookeeperStatefulset(cluster spec.ZookeeperCluster) *appsv1Beta2.StatefulSet {

	name := cluster.ObjectMeta.Name
	replicas := cluster.Spec.BrokerCount

	cpus, err := resource.ParseQuantity(cluster.Spec.Resources.CPU)
	if err != nil {
		cpus, _ = resource.ParseQuantity(defaultCPU)
	}

	memory, err := resource.ParseQuantity(cluster.Spec.Resources.Memory)
	if err != nil {
		memory, _ = resource.ParseQuantity(defaultMemory)
	}

	statefulSet := &appsv1Beta2.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: createLabels(cluster),
			Namespace: cluster.ObjectMeta.Namespace,
		},
		Spec: appsv1Beta2.StatefulSetSpec{
			Replicas: &replicas,

			Selector: &metav1.LabelSelector{
				MatchLabels: createLabels(cluster),
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: createLabels(cluster),
				},
				Spec: v1.PodSpec{
					Affinity: &v1.Affinity{
						PodAntiAffinity: &v1.PodAntiAffinity{
							PreferredDuringSchedulingIgnoredDuringExecution: []v1.WeightedPodAffinityTerm{
								{
									Weight: 100,
									PodAffinityTerm: v1.PodAffinityTerm{
										Namespaces: []string{cluster.ObjectMeta.Namespace},
										LabelSelector: &metav1.LabelSelector{
											MatchLabels: createLabels(cluster),
										},
										TopologyKey: "kubernetes.io/hostname",
									},
								},
							},
						},
					},
					Containers: []v1.Container{
						{
							Name:  "sysctl-conf",
							Image: "busybox:1.26.2",
							Command: []string{
								"sh",
								"-c",
								"sysctl -w vm.max_map_count=262166 && while true; do sleep 86400; done",
							},
							Resources: v1.ResourceRequirements{
								Limits: v1.ResourceList{
									"cpu":    cpus,
									"memory": memory,
								},
								Requests: v1.ResourceList{
									"cpu":    cpus,
									"memory": memory,
								},
							},
							SecurityContext: &v1.SecurityContext{
								Privileged: &[]bool{true}[0],
							},
						},
					},
				},
			},
		},
	}

	return statefulSet;
}

func (k *Kubernetes) CreateOrUpdateStatefulSet(statefulset *appsv1Beta2.StatefulSet) error {
	methodLogger := logger.WithFields(log.Fields{
		"method":    "CreateOrUpdateStatefulSet",
		"name":      statefulset.ObjectMeta.Name,
		"namespace": statefulset.ObjectMeta.Namespace,
	})

	exists, err := k.IfStatefulSetExists(statefulset)
	if err != nil {
		methodLogger.WithField("error", err).Error("Error while checking if statefulsets exists")
		return err
	}

	if !exists {
		err = k.createStatefulSet(statefulset)
	} else {
		err = k.updateStatefulSet(statefulset)
	}
	if err != nil {
		methodLogger.WithField("error", err).Error("Error while creating or updating statefulset")
	}
	return err
}

func (k *Kubernetes) IfStatefulSetExists(statefulset *appsv1Beta2.StatefulSet) (bool, error) {
	methodLogger := logger.WithFields(log.Fields{
		"method":    "IfStatefulSetExists",
		"name":      statefulset.ObjectMeta.Name,
		"namespace": statefulset.ObjectMeta.Namespace,
	})
	namespace := statefulset.ObjectMeta.Namespace
	sts, err := k.Client.AppsV1beta2().StatefulSets(namespace).Get(statefulset.ObjectMeta.Name, k.DefaultOption)

	if err != nil {
		if errors.IsNotFound(err) {
			methodLogger.Debug("StatefulSet doesn't exist")
			return false, nil
		} else {
			methodLogger.WithFields(log.Fields{
				"error": err,
			}).Error("Cant get StatefulSet INFO from API")
			return false, err
		}

	}
	if len(sts.Name) == 0 {
		methodLogger.Debug("StatefulSet.Name == 0, therefore it doesn't exists")
		return false, nil
	}
	return true, nil
}

func (k *Kubernetes) createStatefulSet(statefulset *appsv1Beta2.StatefulSet) error {
	_, err := k.Client.AppsV1beta2().StatefulSets(statefulset.ObjectMeta.Namespace).Create(statefulset)
	return err
}

func (k *Kubernetes) updateStatefulSet(statefulset *appsv1Beta2.StatefulSet) error {
	_, err := k.Client.AppsV1beta2().StatefulSets(statefulset.ObjectMeta.Namespace).Update(statefulset)
	return err
}

func (k *Kubernetes) DeleteStatefulset(statefulset *appsv1Beta2.StatefulSet) error {
	methodLogger := logger.WithFields(log.Fields{
		"method":    "DeleteStatefulset",
		"name":      statefulset.ObjectMeta.Name,
		"namespace": statefulset.ObjectMeta.Namespace,
	})
	exists, err := k.IfStatefulSetExists(statefulset)
	if err != nil {
		methodLogger.WithField("error", err).Error("Error while checking if StatefulSet exists")
		return err
	}
	if exists {
		//Scale the statefulset down to zero (https://github.com/kubernetes/client-go/issues/91)
		replicas := int32(0)
		statefulset.Spec.Replicas = &replicas
		err = k.updateStatefulSet(statefulset)
		if err != nil {
			methodLogger.WithField("error", err).Warn("Error while scaling StatefulSet down to 0, ignoring since deleting afterwards")
		}
		methodLogger.Info("Sleeping 15s per Broker to let StatefulSet scale down ")
		time.Sleep(time.Duration(int(*statefulset.Spec.Replicas)) * time.Second * 15)
		err = k.deleteStatefulSet(statefulset)
		if err != nil {
			methodLogger.WithField("error", err).Error("Can delete statefulset")
			return err
		}
	} else {
		methodLogger.Debug("Trying to delete but Statefulset dosnt exist.")

	}
	return nil
}

func (k *Kubernetes) deleteStatefulSet(statefulset *appsv1Beta2.StatefulSet) error {
	var gracePeriod int64
	gracePeriod = 1

	deleteOption := metav1.DeleteOptions{
		GracePeriodSeconds: &gracePeriod,
	}
	err := k.Client.AppsV1beta1().StatefulSets(statefulset.ObjectMeta.Namespace).Delete(statefulset.ObjectMeta.Name, &deleteOption)

	return err
}

func createLabels(cluster spec.ZookeeperCluster) map[string]string {
	labels := map[string]string{
		"component": "zookeeper",
		"creator":   "zookeeper-operator",
		"role":      "broker",
		"name":      cluster.ObjectMeta.Name,
	}
	return labels
}