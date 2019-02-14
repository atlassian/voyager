package replication

import (
	"context"
	"strconv"

	"github.com/ash2k/stager"
	"github.com/atlassian/ctrl"
	"github.com/atlassian/smith/pkg/specchecker"
	"github.com/atlassian/smith/pkg/store"
	comp_v1 "github.com/atlassian/voyager/pkg/apis/composition/v1"
	"github.com/atlassian/voyager/pkg/composition/client"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/cache"
)

type Controller struct {
	logger         *zap.Logger
	readyForWork   func()
	localInformer  cache.SharedIndexInformer
	remoteInformer cache.SharedIndexInformer
	localClient    client.Interface
}

func (c *Controller) Run(ctx context.Context) {
	defer c.logger.Info("Shutting down Service Descriptor Replicator")
	c.logger.Info("Starting the Service Descriptor Replicator")

	stgr := stager.New()
	defer stgr.Shutdown()
	stage := stgr.NextStage()
	stage.StartWithChannel(c.localInformer.Run)

	// event handler is attached in the constructor
	successful := cache.WaitForCacheSync(ctx.Done(), c.localInformer.HasSynced)
	if !successful {
		c.logger.Error("Could not sync informer cache for Replicator")
		return
	}

	c.readyForWork()
	<-ctx.Done()
}

func shouldReplicate(sd *comp_v1.ServiceDescriptor) (bool, error) {
	replicateStr, ok := sd.Annotations[ReplicateKey]
	if !ok {
		return true, nil
	}

	replicate, err := strconv.ParseBool(replicateStr)
	if err != nil {
		return false, errors.WithStack(err)
	}

	return replicate, nil
}

// TODO: We need to develop a way to reconcile clusters in the many error states we can get into
func (c *Controller) Process(pctx *ctrl.ProcessContext) (bool /* retriable */, error) {
	c.logger.Sugar().Debug("Fetching remote ServiceDescriptor")
	desired := pctx.Object.(*comp_v1.ServiceDescriptor)

	c.logger.Sugar().Debugf("Testing existance of local ServiceDescriptor", desired.GetName())
	existing, exists, err := fetchServiceDescriptor(c.localInformer, desired)
	if err != nil {
		return true, err
	}

	if desired.ObjectMeta.DeletionTimestamp != nil {
		c.logger.Sugar().Debugf("Remote ServiceDescriptor %q marked for deletion", desired.GetName())
		if !exists {
			return false, nil
		}
		// We don't do anything in this case yet
		// https://extranet.atlassian.com/display/VDEV/Soft+deletes
		c.logger.Sugar().Infof("ServiceDescriptor %q should have been deleted, but has been skipped until Soft Delete is implemented", desired.GetName())
		return false, nil
	}

	// Check if we need to replicate
	desiredShouldReplicate, err := shouldReplicate(desired)
	if err != nil {
		return false, err
	}
	if !desiredShouldReplicate {
		c.logger.Sugar().Infof("Remote ServiceDescriptor %q is explicitly marked as non-replicating and will not be copied", desired.GetName())
		return false, nil
	}

	// Create the local SD
	if exists {
		existingShouldReplicate, shouldReplicateErr := shouldReplicate(existing)
		if shouldReplicateErr != nil {
			return false, shouldReplicateErr
		}
		if !existingShouldReplicate {
			c.logger.Sugar().Infof("ServiceDescriptor %q is explicitly marked as non-replicating and will not be overwritten", existing.GetName())
			return false, nil
		}

		return c.updateServiceDescriptor(pctx, existing, desired)
	}
	desired = stripResourceVersion(desired)
	return c.createServiceDescriptor(pctx, desired)
}

func fetchServiceDescriptor(inf cache.SharedIndexInformer, item *comp_v1.ServiceDescriptor) (*comp_v1.ServiceDescriptor, bool, error) {
	obj, exists, err := inf.GetIndexer().Get(item)
	if exists {
		sd := obj.(*comp_v1.ServiceDescriptor).DeepCopy()
		// Typed objects have their TypeMeta erased. Put it back.
		sd.SetGroupVersionKind(comp_v1.ServiceDescriptorGVK)
		return sd, exists, err
	}
	return nil, exists, err
}

func (c *Controller) createServiceDescriptor(pctx *ctrl.ProcessContext, desired *comp_v1.ServiceDescriptor) (bool, error) {
	pctx.Logger.Sugar().Infof("Creating ServiceDescriptor %q", desired.GetName())
	_, err := c.localClient.CompositionV1().ServiceDescriptors().Create(desired)
	if err != nil {
		return true, err
	}
	return false, nil
}

func (c *Controller) updateServiceDescriptor(pctx *ctrl.ProcessContext, existing, desired *comp_v1.ServiceDescriptor) (bool, error) {
	store := store.NewMultiBasic()
	sc := specchecker.New(store)

	_, equal, _, err := sc.CompareActualVsSpec(pctx.Logger, desired, existing)
	if err != nil {
		return false, err
	}
	if equal {
		pctx.Logger.Sugar().Infof("No updates to ServiceDescriptor %q", desired.GetName())
		return false, nil
	}
	pctx.Logger.Sugar().Infof("Updating ServiceDescriptor %q", desired.GetName())

	err = validateHash(existing)
	if err != nil {
		return false, errors.Errorf("aborting replication of %q due to hash check error: %q", desired.GetName(), err)
	}

	updated := existing.DeepCopy()
	updated.Spec = desired.Spec
	for key, value := range desired.Labels {
		updated.Labels[key] = value
	}
	for key, value := range desired.Annotations {
		updated.Annotations[key] = value
	}
	finalizers := sets.NewString(updated.GetFinalizers()...)
	finalizers.Insert(desired.GetFinalizers()...)
	updated.SetFinalizers(finalizers.List())

	_, err = c.localClient.CompositionV1().ServiceDescriptors().Update(updated)
	if err != nil {
		return true, err
	}
	return false, nil
}

func validateHash(existing *comp_v1.ServiceDescriptor) error {
	hash, exists := existing.Annotations[hashKey]
	if !exists {
		// Weird, but something has wiped the "dirty bit" hash or it never existed
		return nil
	}

	regen, err := generateSpecHash(existing)
	if err != nil {
		return err
	}

	if hash != regen {
		return errors.New("hash does not match - user mutation detected")
	}

	return nil
}

// This value is only valid within a cluster, it should not be used cross cluster for object creation
func stripResourceVersion(existing *comp_v1.ServiceDescriptor) *comp_v1.ServiceDescriptor {
	existing.ObjectMeta.ResourceVersion = ""
	return existing
}
