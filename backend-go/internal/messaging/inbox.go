package messaging

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

type InboxStore struct{ DB *sql.DB }

func NewInboxStore(db *sql.DB) *InboxStore { return &InboxStore{DB: db} }

// ClaimTx records an unprocessed delivery in a short transaction. An existing
// processed marker is a duplicate; an unprocessed marker is reclaimable after
// a worker crash.
func (s *InboxStore) ClaimTx(ctx context.Context, tx *sql.Tx, consumerName, eventID string) (bool, error) {
	if s == nil || tx == nil || consumerName == "" || eventID == "" {
		return false, fmt.Errorf("invalid inbox claim arguments")
	}
	var inserted bool
	err := tx.QueryRowContext(ctx, `INSERT INTO consumer_inbox (consumer_name,event_id,processed_at) VALUES ($1,$2,NULL) ON CONFLICT (consumer_name,event_id) DO NOTHING RETURNING true`, consumerName, eventID).Scan(&inserted)
	if err == nil {
		return false, nil
	}
	if err != sql.ErrNoRows {
		return false, err
	}
	var processedAt sql.NullTime
	if err := tx.QueryRowContext(ctx, `SELECT processed_at FROM consumer_inbox WHERE consumer_name=$1 AND event_id=$2 FOR UPDATE`, consumerName, eventID).Scan(&processedAt); err != nil {
		return false, err
	}
	return processedAt.Valid, nil
}

func (s *InboxStore) CompleteTx(ctx context.Context, tx *sql.Tx, consumerName, eventID, result string, metadata map[string]any) error {
	if s == nil || tx == nil || consumerName == "" || eventID == "" {
		return fmt.Errorf("invalid inbox completion arguments")
	}
	var encoded []byte
	var err error
	if metadata != nil {
		encoded, err = json.Marshal(metadata)
		if err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE consumer_inbox SET processed_at=now(),result=$1,metadata=$2,error_message=NULL WHERE consumer_name=$3 AND event_id=$4`, result, encoded, consumerName, eventID)
	return err
}

// ProcessTx inserts the inbox marker and invokes handler in the same DB
// transaction. A failed handler leaves no committed marker, so delivery can
// safely be retried. The caller commits/rolls back tx.
func (s *InboxStore) ProcessTx(ctx context.Context, tx *sql.Tx, consumerName, eventID string, handler func(context.Context) (result string, metadata map[string]any, err error)) (duplicate bool, err error) {
	if s == nil || tx == nil || consumerName == "" || eventID == "" || handler == nil {
		return false, fmt.Errorf("invalid inbox process arguments")
	}
	var inserted bool
	if err := tx.QueryRowContext(ctx, `INSERT INTO consumer_inbox (consumer_name,event_id,processed_at) VALUES ($1,$2,NULL) ON CONFLICT (consumer_name,event_id) DO NOTHING RETURNING true`, consumerName, eventID).Scan(&inserted); err != nil {
		if err == sql.ErrNoRows {
			return true, nil
		}
		return false, err
	}
	result, metadata, err := handler(ctx)
	if err != nil {
		return false, err
	}
	var encoded []byte
	if metadata != nil {
		encoded, err = json.Marshal(metadata)
		if err != nil {
			return false, err
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE consumer_inbox SET processed_at=now(),result=$1,metadata=$2,error_message=NULL WHERE consumer_name=$3 AND event_id=$4`, result, encoded, consumerName, eventID)
	return false, err
}
