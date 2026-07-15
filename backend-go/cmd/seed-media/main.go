package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type demoAsset struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	SlotKey    string `json:"slotKey"`
	CategoryID string `json:"categoryId"`
	From       string `json:"from"`
	To         string `json:"to"`
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	manifestPath := env("MEDIA_DEMO_MANIFEST", "static/demo-assets/manifest.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		log.Fatal(err)
	}
	var items []demoAsset
	if err = json.Unmarshal(raw, &items); err != nil {
		log.Fatal(err)
	}
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	if err = db.PingContext(ctx); err != nil {
		log.Fatal(err)
	}
	root := env("MEDIA_STORAGE_ROOT", "data/media-assets")
	publicBase := strings.TrimRight(env("MEDIA_PUBLIC_BASE_URL", "/api/v1/media/files"), "/")
	for _, item := range items {
		svg := demoSVG(item)
		sum := sha256.Sum256(svg)
		hash := hex.EncodeToString(sum[:])
		key := filepath.ToSlash(filepath.Join("tenant", "default", "demo", hash[:32]+".svg"))
		target := filepath.Join(root, filepath.FromSlash(key))
		if err = os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			log.Fatal(err)
		}
		if _, err = os.Stat(target); os.IsNotExist(err) {
			if err = os.WriteFile(target, svg, 0o644); err != nil {
				log.Fatal(err)
			}
		}
		url := publicBase + "/" + key
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			log.Fatal(err)
		}
		_, err = tx.ExecContext(ctx, `insert into xz_media_assets(id,tenant_id,name,category_id,asset_type,mime_type,file_ext,original_name,storage_provider,storage_key,original_url,cdn_url,thumbnail_url,width,height,aspect_ratio,file_size,file_hash,status,audit_status,source_type,source_name,license_type,license_note,created_by,updated_by) values($1,'default',$2,nullif($3,''),'IMAGE','image/svg+xml','svg',$4,'local',$5,$6,$6,$6,750,440,1.704545,$7,$8,'ACTIVE','APPROVED','SYSTEM_SEED','知启云AI 测试环境','INTERNAL_DEMO','仅用于测试环境初始化','system','system') on conflict(tenant_id,file_hash) do update set name=excluded.name,category_id=excluded.category_id,cdn_url=excluded.cdn_url,thumbnail_url=excluded.thumbnail_url,deleted_at=null,updated_at=now()`, item.ID, item.Name, item.CategoryID, item.ID+".svg", key, url, len(svg), hash)
		if err == nil {
			_, err = tx.ExecContext(ctx, `update xz_page_asset_slots set asset_id=$1,updated_at=now() where tenant_id='default' and slot_key=$2`, item.ID, item.SlotKey)
		}
		if err == nil {
			_, err = tx.ExecContext(ctx, `insert into xz_media_asset_usage(id,tenant_id,asset_id,page_code,module_code,slot_key,business_type,business_id) select 'usage_seed_'||$1,'default',$1,page_code,module_code,slot_key,'PAGE_SLOT',id from xz_page_asset_slots where tenant_id='default' and slot_key=$2 on conflict do nothing`, item.ID, item.SlotKey)
		}
		if err == nil {
			_, err = tx.ExecContext(ctx, `update xz_media_assets set usage_count=(select count(*) from xz_media_asset_usage where tenant_id='default' and asset_id=$1) where tenant_id='default' and id=$1`, item.ID)
		}
		if err != nil {
			_ = tx.Rollback()
			log.Fatal(err)
		}
		if err = tx.Commit(); err != nil {
			log.Fatal(err)
		}
		log.Printf("seeded %s -> %s", item.Name, item.SlotKey)
	}
}
func demoSVG(item demoAsset) []byte {
	return []byte(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="750" height="440" viewBox="0 0 750 440"><defs><linearGradient id="g" x2="1" y2="1"><stop stop-color="%s"/><stop offset="1" stop-color="%s"/></linearGradient></defs><rect width="750" height="440" rx="36" fill="url(#g)"/><circle cx="625" cy="95" r="145" fill="#fff" opacity=".13"/><circle cx="565" cy="365" r="110" fill="#fff" opacity=".09"/><path d="M90 310c92-116 174-116 246 0 80-98 165-98 255 0" fill="none" stroke="#fff" stroke-width="24" stroke-linecap="round" opacity=".28"/><text x="56" y="142" font-family="sans-serif" font-size="26" fill="#fff" opacity=".8">知启云 AI</text><text x="56" y="210" font-family="sans-serif" font-size="48" font-weight="700" fill="#fff">%s</text><text x="56" y="260" font-family="sans-serif" font-size="22" fill="#fff" opacity=".82">测试环境演示素材 · 可在素材中心替换</text></svg>`, item.From, item.To, html.EscapeString(item.Name)))
}
func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
