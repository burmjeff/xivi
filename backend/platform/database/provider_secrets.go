package database

import (
	"context"
	"fmt"
	"xivi/backend/pkg/security"

	"github.com/jmoiron/sqlx"
)

type providerURLRow struct {
	ID  int64  `db:"id"`
	URL string `db:"url"`
}

type encryptedProviderURLRow struct {
	ID     int64  `db:"id"`
	URL    string `db:"url"`
	Cipher []byte `db:"url_cipher"`
}

// encryptExistingProviderURLs performs the one-way cutover from legacy
// plaintext provider URLs. It is deliberately part of startup: Xivi will not
// begin serving requests if any populated provider credential cannot be moved
// under the mounted application key.
func encryptExistingProviderURLs(ctx context.Context, db *sqlx.DB) error {
	return withImmediateTransaction(ctx, db, func(tx *sqlx.Tx) error {
		for _, table := range []string{"playlist", "epg", "channelurl"} {
			// table is selected exclusively from this server-owned constant list.
			rows := []providerURLRow{}
			if err := tx.SelectContext(ctx, &rows, `SELECT id, url FROM `+table+`
				WHERE url_cipher IS NULL AND url IS NOT NULL AND url != ''`); err != nil {
				return fmt.Errorf("load %s provider URLs: %w", table, err)
			}
			for _, row := range rows {
				ciphertext, err := security.EncryptSecret([]byte(row.URL))
				if err != nil {
					return err
				}
				result, err := tx.ExecContext(ctx, `UPDATE `+table+`
					SET url = 'encrypted', url_cipher = ?
					WHERE id = ? AND url_cipher IS NULL`, ciphertext, row.ID)
				if err != nil {
					return fmt.Errorf("encrypt %s provider URL: %w", table, err)
				}
				if changed, _ := result.RowsAffected(); changed != 1 {
					return fmt.Errorf("encrypt %s provider URL: row changed concurrently", table)
				}
			}

			encryptedRows := []encryptedProviderURLRow{}
			if err := tx.SelectContext(ctx, &encryptedRows, `SELECT id, url, url_cipher FROM `+table+`
				WHERE url_cipher IS NOT NULL`); err != nil {
				return fmt.Errorf("verify %s provider URLs: %w", table, err)
			}
			for _, row := range encryptedRows {
				if row.URL != "encrypted" {
					return fmt.Errorf("verify %s provider URL %d: plaintext marker is invalid", table, row.ID)
				}
				if _, err := security.DecryptSecret(row.Cipher); err != nil {
					return fmt.Errorf("verify %s provider URL %d: %w", table, row.ID, err)
				}
			}
		}
		logos := []providerURLRow{}
		if err := tx.SelectContext(ctx, &logos, `SELECT id, tvg_logo AS url FROM playlistchannel
			WHERE tvg_logo IS NOT NULL AND TRIM(tvg_logo) != ''`); err != nil {
			return fmt.Errorf("load provider logo URLs: %w", err)
		}
		for _, row := range logos {
			protected, err := security.ProtectString(row.URL)
			if err != nil {
				return fmt.Errorf("protect provider logo URL %d: %w", row.ID, err)
			}
			if protected != row.URL {
				if _, err := tx.ExecContext(ctx, `UPDATE playlistchannel SET tvg_logo = ? WHERE id = ?`, protected, row.ID); err != nil {
					return fmt.Errorf("protect provider logo URL %d: %w", row.ID, err)
				}
			}
		}
		// External XMLTV artwork is not used by the product UI and cannot safely
		// cross the credential boundary. Remove legacy values rather than retain
		// signed provider URLs in database exports.
		for _, table := range []string{"epgchannel", "epgprogramme"} {
			if _, err := tx.ExecContext(ctx, `UPDATE `+table+` SET "icon.src" = ''
				WHERE "icon.src" IS NOT NULL AND TRIM("icon.src") != ''`); err != nil {
				return fmt.Errorf("remove external %s artwork: %w", table, err)
			}
		}
		mfaSeeds := []struct {
			UserID int64  `db:"user_id"`
			Secret []byte `db:"encrypted_secret"`
		}{}
		if err := tx.SelectContext(ctx, &mfaSeeds, `SELECT user_id, encrypted_secret FROM user_mfa`); err != nil {
			return fmt.Errorf("verify encrypted MFA secrets: %w", err)
		}
		for _, seed := range mfaSeeds {
			if _, err := security.DecryptSecret(seed.Secret); err != nil {
				return fmt.Errorf("verify encrypted MFA secret for user %d: %w", seed.UserID, err)
			}
		}
		return nil
	})
}

func withImmediateTransaction(ctx context.Context, db *sqlx.DB, operation func(*sqlx.Tx) error) error {
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := operation(tx); err != nil {
		return err
	}
	return tx.Commit()
}
