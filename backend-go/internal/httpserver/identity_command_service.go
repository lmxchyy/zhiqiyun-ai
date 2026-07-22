package httpserver

import "errors"

// identityCommandService is the only HTTP-facing entry point for commercial
// identity mutations. Transport code must not compose profile, relationship,
// order, ledger or RBAC writes itself.
type identityCommandService struct {
	store platformStore
}

func newIdentityCommandService(store platformStore) *identityCommandService {
	return &identityCommandService{store: store}
}

func (s *identityCommandService) Preview(actorID, actorRole, userID string, request identityChangePreviewRequest) (identityChangePreviewResult, error) {
	store, ok := s.store.(adminIdentityChangeStore)
	if !ok {
		return identityChangePreviewResult{}, errors.New("identity command store is unavailable")
	}
	return store.PreviewAdminIdentityChange(actorID, actorRole, userID, request)
}

func (s *identityCommandService) Review(actorID, actorRole, userID string, request identityChangeReviewRequest) (identityChangePreviewResult, error) {
	store, ok := s.store.(adminIdentityChangeStore)
	if !ok {
		return identityChangePreviewResult{}, errors.New("identity command store is unavailable")
	}
	return store.ReviewAdminIdentityChange(actorID, actorRole, userID, request)
}

func (s *identityCommandService) Confirm(actorID, actorRole, userID string, request identityChangeConfirmRequest) (identityChangeConfirmResult, error) {
	store, ok := s.store.(adminIdentityChangeStore)
	if !ok {
		return identityChangeConfirmResult{}, errors.New("identity command store is unavailable")
	}
	return store.ConfirmAdminIdentityChange(actorID, actorRole, userID, request)
}

func (s *identityCommandService) PreviewDowngrade(actorID, actorRole, userID string, request identityDowngradeRequest) (identityDowngradePreview, error) {
	store, ok := s.store.(adminIdentityDowngradeStore)
	if !ok {
		return identityDowngradePreview{}, errors.New("identity downgrade command store is unavailable")
	}
	return store.PreviewAdminIdentityDowngrade(actorID, actorRole, userID, request)
}

func (s *identityCommandService) ConfirmDowngrade(actorID, actorRole, userID string, request identityDowngradeConfirmRequest) (identityDowngradeResult, error) {
	store, ok := s.store.(adminIdentityDowngradeStore)
	if !ok {
		return identityDowngradeResult{}, errors.New("identity downgrade command store is unavailable")
	}
	return store.ConfirmAdminIdentityDowngrade(actorID, actorRole, userID, request)
}

func (s *identityCommandService) ListDowngrades(actorID, actorRole, userID string) ([]identityDowngradeResult, error) {
	store, ok := s.store.(adminIdentityDowngradeStore)
	if !ok {
		return nil, errors.New("identity downgrade command store is unavailable")
	}
	return store.ListAdminIdentityDowngrades(actorID, actorRole, userID)
}

func (s *identityCommandService) RecheckDowngrade(actorID, actorRole, userID, requestID string) (identityDowngradeResult, error) {
	store, ok := s.store.(adminIdentityDowngradeStore)
	if !ok {
		return identityDowngradeResult{}, errors.New("identity downgrade command store is unavailable")
	}
	return store.RecheckAdminIdentityDowngrade(actorID, actorRole, userID, requestID)
}

func (s *identityCommandService) CancelDowngrade(actorID, actorRole, userID, requestID string) (identityDowngradeResult, error) {
	store, ok := s.store.(adminIdentityDowngradeStore)
	if !ok {
		return identityDowngradeResult{}, errors.New("identity downgrade command store is unavailable")
	}
	return store.CancelAdminIdentityDowngrade(actorID, actorRole, userID, requestID)
}

func (s *identityCommandService) RescheduleDowngrade(actorID, actorRole, userID, requestID string, request identityDowngradeRescheduleRequest) (identityDowngradeResult, error) {
	store, ok := s.store.(adminIdentityDowngradeStore)
	if !ok {
		return identityDowngradeResult{}, errors.New("identity downgrade command store is unavailable")
	}
	return store.RescheduleAdminIdentityDowngrade(actorID, actorRole, userID, requestID, request)
}
