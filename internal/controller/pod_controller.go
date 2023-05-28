package controller

import (
	"context"
	"fmt"

	"Time-k8s-controller/tools/statussync"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type PodReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	statusInformer *statussync.StatusInformer
}

func NewPodReconciler(client client.Client, scheme *runtime.Scheme, statusInformer *statussync.StatusInformer) *PodReconciler {
	return &PodReconciler{
		client,
		scheme,
		statusInformer,
	}
}

//+kubebuilder:rbac:groups=k8s-controller.my.domain,resources=pods,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=k8s-controller.my.domain,resources=pods/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=k8s-controller.my.domain,resources=pods/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Pod object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.4/pkg/reconcile
func (r *PodReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	// TODO(user): your logic here
	pod := &v1.Pod{}
	err := r.Client.Get(ctx, req.NamespacedName, pod)
	if err != nil {
		if !errors.IsNotFound(err) {
			klog.Errorln(err, "get pod")
			return ctrl.Result{Requeue: true}, err
		}

		return ctrl.Result{}, nil
	}
	fmt.Printf("name:%s, status:%s\n", pod.Name, pod.Status.Phase)
	// 通知对端Pod已经处于Running状态
	if pod.Status.Phase == v1.PodRunning {
		r.statusInformer.Sync(pod.Name)
	}
	return ctrl.Result{}, nil
}

func (r *PodReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		// Uncomment the following line adding a pointer to an instance of the controlled resource as an argument
		For(&v1.Pod{}).
		Complete(r)
}
