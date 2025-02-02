package masteruserrecord

import (
	"context"
	toolchainv1alpha1 "github.com/codeready-toolchain/api/pkg/apis/toolchain/v1alpha1"
	"github.com/codeready-toolchain/toolchain-common/pkg/condition"
	corev1 "k8s.io/api/core/v1"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Synchronizer struct {
	hostClient        client.Client
	memberClient      client.Client
	memberUserAcc     *toolchainv1alpha1.UserAccount
	recordSpecUserAcc toolchainv1alpha1.UserAccountEmbedded
	record            *toolchainv1alpha1.MasterUserRecord
}

func (s *Synchronizer) synchronizeSpec() error {
	if !reflect.DeepEqual(s.memberUserAcc.Spec, s.recordSpecUserAcc.Spec) {
		// when UserAccount spec in record is updated - is not same as in member
		s.memberUserAcc.Spec = s.recordSpecUserAcc.Spec
		if err := updateStatusConditions(s.hostClient, s.record, toBeNotReady(updatingReason, "")); err != nil {
			return err
		}
		err := s.memberClient.Update(context.TODO(), s.memberUserAcc)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Synchronizer) synchronizeStatus() error {
	recordStatusUserAcc, index := getUserAccountStatus(s.recordSpecUserAcc.TargetCluster, s.record)
	if index < 0 || s.recordSpecUserAcc.SyncIndex != recordStatusUserAcc.SyncIndex {
		// when record should update status
		recordStatusUserAcc.SyncIndex = s.recordSpecUserAcc.SyncIndex
		recordStatusUserAcc.UserAccountStatus = s.memberUserAcc.Status
		var originalStatusUserAcc toolchainv1alpha1.UserAccountStatusEmbedded
		if index < 0 {
			s.record.Status.UserAccounts = append(s.record.Status.UserAccounts, recordStatusUserAcc)
		} else {
			originalStatusUserAcc = s.record.Status.UserAccounts[index]
			s.record.Status.UserAccounts[index] = recordStatusUserAcc
		}

		s.alignReadiness()

		err := s.hostClient.Status().Update(context.TODO(), s.record)
		if err != nil {
			if index < 0 {
				s.record.Status.UserAccounts = s.record.Status.UserAccounts[:len(s.record.Status.UserAccounts)-1]
			} else {
				s.record.Status.UserAccounts[index] = originalStatusUserAcc
			}
			return err
		}
	}
	return nil
}

// alignReadiness checks if all embedded SAs are ready
func (s *Synchronizer) alignReadiness() {
	for _, uaStatus := range s.record.Status.UserAccounts {
		if !isReady(uaStatus.Conditions) {
			return
		}
	}
	s.record.Status.Conditions, _ = condition.AddOrUpdateStatusConditions(s.record.Status.Conditions, toBeProvisioned())
}

func isReady(conditions []toolchainv1alpha1.Condition) bool {
	for _, con := range conditions {
		if con.Type == toolchainv1alpha1.ConditionReady {
			return con.Status == corev1.ConditionTrue
		}
	}
	return false
}

func getUserAccountStatus(clusterName string, record *toolchainv1alpha1.MasterUserRecord) (toolchainv1alpha1.UserAccountStatusEmbedded, int) {
	for i, account := range record.Status.UserAccounts {
		if account.TargetCluster == clusterName {
			return account, i
		}
	}
	return toolchainv1alpha1.UserAccountStatusEmbedded{
		TargetCluster: clusterName,
	}, -1
}
