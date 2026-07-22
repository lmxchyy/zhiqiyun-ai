package httpserver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

func identityActorHasPermissionTx(ctx context.Context, tx *sql.Tx, actorID, actorRole, permission string) (bool, error) {
	var allowed bool
	err := tx.QueryRowContext(ctx, `
		SELECT EXISTS(
		  SELECT 1 FROM xz_role_permissions permission_binding
		  WHERE permission_binding.permission=$3
		    AND permission_binding.role=upper($2)
		) OR EXISTS(
		  SELECT 1
		  FROM xz_user_roles role_binding
		  JOIN xz_role_permissions permission_binding ON permission_binding.role=role_binding.role
		  WHERE role_binding.user_id=$1 AND upper(role_binding.status)='ACTIVE'
		    AND permission_binding.permission=$3
		)
	`, actorID, actorRole, permission).Scan(&allowed)
	return allowed, err
}

func validateIdentityPaymentProofTx(ctx context.Context, tx *sql.Tx, proof identityPaymentProof) error {
	if strings.TrimSpace(proof.Reference) == "" || strings.TrimSpace(proof.PayerName) == "" || strings.TrimSpace(proof.PaymentChannel) == "" || strings.TrimSpace(proof.PaidAt) == "" {
		return errors.New("payment proof requires reference, payerName, paidAt and paymentChannel")
	}
	if _, err := time.Parse(time.RFC3339, proof.PaidAt); err != nil {
		if _, nanoErr := time.Parse(time.RFC3339Nano, proof.PaidAt); nanoErr != nil {
			return errors.New("payment proof paidAt must be an RFC3339 timestamp")
		}
	}
	if proof.StorageFileID == "" {
		return nil
	}
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM xz_file_objects WHERE file_id=$1 AND status='ACTIVE')`, proof.StorageFileID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("payment proof storageFileId is not an active storage object")
	}
	return nil
}
