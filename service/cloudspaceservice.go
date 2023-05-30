package service

import (
	"Time-k8s-controller/pb"
	"Time-k8s-controller/tools/statussync"
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const ModeRelease = "development"

var (
	ResponseSuccess = &pb.Response{Status: 200, Message: "success"}
	ResponseFailed  = &pb.Response{Status: 400, Message: "failed"}
)
var (
	EmptyWorkspaceRunningInfo = &pb.WorkspaceRunningInfo{}
	EmptyResponse             = &pb.Response{}
	EmptyWorkspaceInfo        = &pb.WorkspaceInfo{}
	EmptyWorkspaceStatus      = &pb.WorkspaceStatus{}
)

const (
	PodNotExist int32 = iota
	PodExist
)

var Mode string
var podTpl = &v1.Pod{
	TypeMeta: metav1.TypeMeta{
		APIVersion: "v1",
		Kind:       "Pod",
	},
	ObjectMeta: metav1.ObjectMeta{
		Labels: map[string]string{
			"kind": "k8s-test",
		},
	},
}

type SpaceService struct {
	Client         client.Client
	StatusInformer *statussync.StatusInformer
}

func NewSpaceService(client client.Client, manager *statussync.StatusInformer) *SpaceService {
	return &SpaceService{
		Client:         client,
		StatusInformer: manager,
	}
}

func (ss *SpaceService) CreateSpace(ctx context.Context, info *pb.WorkspaceInfo) (*pb.WorkspaceRunningInfo, error) {
	pvc_name := info.Name
	pvc, err := ss.CreatePVC(pvc_name, info.Namespace, info.ResourceLimit.Storage)
	if err != nil {
		klog.Errorf("construct pvc error:%v, info:%v", err, info)
		return EmptyWorkspaceRunningInfo, status.Error(codes.Unknown, ErrConstructPVC.Error())
	}
	klog.Infoln("create pvc success")
	deadline, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()
	err = ss.Client.Create(deadline, pvc)
	if err != nil {
		// 如果PVC已经存在
		if errors.IsAlreadyExists(err) {
			klog.Infof("create pvc while pvc is already exist, pvc:%s", pvc_name)
		} else {
			klog.Errorf("create pvc error:%v", err)
			return EmptyWorkspaceRunningInfo, status.Error(codes.Unknown, ErrConstructPVC.Error())
		}
	}
	klog.Info("[CreateSpace] 2.create pvc success")

	// 2.创建Pod
	return ss.CreatePod(ctx, info)
}

func (ss *SpaceService) CreatePVC(name, namespace, storage string) (*v1.PersistentVolumeClaim, error) {
	fmt.Println(storage, "----------------")
	q, err := resource.ParseQuantity(storage)
	if err != nil {
		fmt.Println(err)
		return nil, err
	}
	storage_class_name := "nfs-client"
	pvc := &v1.PersistentVolumeClaim{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "PersistentVolumeClaim",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			// Labels: map[string]string{
			// 	"run": "nginx",
			// },
		},
		Spec: v1.PersistentVolumeClaimSpec{
			AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteMany},
			Resources: v1.ResourceRequirements{
				Limits: v1.ResourceList{
					v1.ResourceStorage: q,
				},
				Requests: v1.ResourceList{
					v1.ResourceStorage: q,
				},
			},
			StorageClassName: &storage_class_name,
		},
	}
	return pvc, nil
}

func (ss *SpaceService) CreatePod(c context.Context, info *pb.WorkspaceInfo) (*pb.WorkspaceRunningInfo, error) {
	pod := podTpl.DeepCopy()
	ss.fillPod(info, pod, info.Name)
	fmt.Println("11111111")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()
	err := ss.Client.Create(ctx, pod)
	if err != nil {
		// 如果该Pod已经存在
		if errors.IsAlreadyExists(err) {
			klog.Infof("create pod while pod is already exist, pod:%s", info.Name)
			// 判断Pod是否处于running状态
			existPod := v1.Pod{}
			err = ss.Client.Get(context.Background(), client.ObjectKeyFromObject(pod), &existPod)
			if err != nil {
				return EmptyWorkspaceRunningInfo, status.Error(codes.Unknown, ErrCreatePod.Error())
			}
			if existPod.Status.Phase == v1.PodRunning {
				return &pb.WorkspaceRunningInfo{
					NodeName: existPod.Spec.NodeName,
					Ip:       existPod.Status.PodIP,
					Port:     existPod.Spec.Containers[0].Ports[0].ContainerPort,
				}, nil
			} else {
				ss.deletePod(&existPod)
				return EmptyWorkspaceRunningInfo, status.Error(codes.Unknown, ErrCreatePod.Error())
			}

		} else {
			klog.Errorf("create pod err:%v", err)
			return EmptyWorkspaceRunningInfo, status.Error(codes.Unknown, ErrCreatePod.Error())
		}
	}
	klog.Info("[createPod] create pod success")
	ch := ss.StatusInformer.Add(pod.Name)
	fmt.Println("333333333333333", ch)
	defer ss.StatusInformer.Delete(pod.Name)
	select {
	case <-ch:
		// Pod已经处于running状态
		return ss.GetPodSpaceInfo(context.Background(), &pb.QueryOption{Name: info.Name, Namespace: info.Namespace})
	case <-c.Done():
		// 超时,Pod启动失败,可能是由于资源不足,将Pod删除
		klog.Error("pod start failed, maybe resources is not enough")
		ss.deletePod(pod)
		return EmptyWorkspaceRunningInfo, status.Error(codes.Unknown, ErrCreatePod.Error())
	}
}

/*
apiVersion: v1
kind: Pod
metadata:

	name: code-server-volum
	namespace: cloud-ide
	labels:
	  kind: code-server

spec:

	containers:
	- name: code-server
	  image: mangohow/code-server-go1.19:v0.1
	  volumeMounts:
	  - name: volume
	    mountPath: /root/workspace
	volumes:
	- name: volume
	  persistentVolumeClaim:
	    claimName: pvc3
	    readOnly: false
*/
func (ss *SpaceService) fillPod(info *pb.WorkspaceInfo, pod *v1.Pod, pvc string) {
	volumeName := "volume-user-space"
	pod.Name = info.Name
	pod.Namespace = info.Namespace
	pod.Spec.Volumes = []v1.Volume{
		v1.Volume{
			VolumeSource: v1.VolumeSource{
				PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
					ClaimName: pvc,
					ReadOnly:  false,
				},
			},
			Name: volumeName,
		},
	}
	pod.Spec.Containers = []v1.Container{
		v1.Container{
			Name:  info.Name,
			Image: info.Image,
			VolumeMounts: []v1.VolumeMount{
				v1.VolumeMount{
					Name:      volumeName,
					MountPath: "/user_data/",
					ReadOnly:  false,
				},
			},
			ImagePullPolicy: v1.PullIfNotPresent,
			Ports: []v1.ContainerPort{
				v1.ContainerPort{
					ContainerPort: int32(info.Port),
				},
			},
		},
	}
	if ModeRelease == "" {
		fmt.Println("============resource=======111======", Mode, "--", ModeRelease)
		pod.Spec.Containers[0].Resources = v1.ResourceRequirements{
			Requests: map[v1.ResourceName]resource.Quantity{
				v1.ResourceCPU:    resource.MustParse("2"),
				v1.ResourceMemory: resource.MustParse("1Gi"),
			},
			Limits: map[v1.ResourceName]resource.Quantity{
				v1.ResourceCPU:    resource.MustParse(info.ResourceLimit.Cpu),
				v1.ResourceMemory: resource.MustParse(info.ResourceLimit.Memory),
			},
		}
	}
}

func (ss *SpaceService) deletePod(pod *v1.Pod) (*pb.Response, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*32)
	defer cancel()
	err := ss.Client.Delete(ctx, pod)
	if err != nil {
		if errors.IsNotFound(err) {
			klog.Infof("delete pod while pod not exist, pod:%s", pod.Name)
			return ResponseSuccess, nil
		}

		klog.Errorf("delete pod error:%v", err)
		return ResponseFailed, status.Error(codes.Unknown, ErrDeletePod.Error())
	}
	klog.Info("[deletePod] delete pod success")

	return ResponseSuccess, nil
}

func (ss *SpaceService) GetPodSpaceInfo(ctx context.Context, option *pb.QueryOption) (*pb.WorkspaceRunningInfo, error) {
	pod := v1.Pod{}
	fmt.Println("2222222222")
	err := ss.Client.Get(ctx, client.ObjectKey{Name: option.Name, Namespace: option.Namespace}, &pod)
	if err != nil {
		if errors.IsNotFound(err) {
			return EmptyWorkspaceRunningInfo, status.Error(codes.NotFound, "pod not found")
		}

		klog.Errorf("get pod space info error:%v", err)
		return EmptyWorkspaceRunningInfo, status.Error(codes.Unknown, err.Error())
	}
	fmt.Println("3333333333")
	return &pb.WorkspaceRunningInfo{
		NodeName: pod.Spec.NodeName,
		Ip:       pod.Status.PodIP,
		Port:     pod.Spec.Containers[0].Ports[0].ContainerPort,
	}, nil
}

// 开启一个空间对应创建一个pod
func (ss *SpaceService) StartSpace(ctx context.Context, info *pb.WorkspaceInfo) (*pb.WorkspaceRunningInfo, error) {
	return ss.CreatePod(ctx, info)
}

func (ss *SpaceService) DeleteSpace(ctx context.Context, option *pb.QueryOption) (*pb.Response, error) {
	pvc := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      option.Name,
			Namespace: option.Namespace,
		},
	}
	c, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()
	err := ss.Client.Delete(c, pvc)
	if err != nil {
		// 如果是PVC不存在引起的错误就认为是成功了,因为就是要删除PVC
		if errors.IsNotFound(err) {
			klog.Infof("pvc not found,err:%v", err)
			return ResponseSuccess, nil
		}
		klog.Errorf("delete pvc error:%v", err)
		return ResponseFailed, status.Error(codes.Unknown, ErrDeletePVC.Error())
	}
	klog.Info("[DeleteSpace] delete pvc success")

	return ResponseSuccess, nil
}

// 删除对应pod
func (ss *SpaceService) StopSpace(ctx context.Context, option *pb.QueryOption) (*pb.Response, error) {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      option.Name,
			Namespace: option.Namespace,
		},
	}
	return ss.deletePod(pod)
}

func (ss *SpaceService) GetPodSpaceStatus(ctx context.Context, option *pb.QueryOption) (*pb.WorkspaceStatus, error) {
	pod := v1.Pod{}
	fmt.Println(option.Name, option.Namespace, "444444444")
	err := ss.Client.Get(ctx, client.ObjectKey{Name: option.Name, Namespace: option.Namespace}, &pod)
	if err != nil {
		if errors.IsNotFound(err) {
			fmt.Println(err, "------")
			return EmptyWorkspaceStatus, status.Error(codes.NotFound, "pod not found")
		}

		klog.Errorf("get pod space status error:%v", err)
		return &pb.WorkspaceStatus{Status: PodNotExist, Message: "NotExist"}, status.Error(codes.Unknown, err.Error())
	}

	return &pb.WorkspaceStatus{Status: PodExist, Message: string(pod.Status.Phase)}, nil
}
