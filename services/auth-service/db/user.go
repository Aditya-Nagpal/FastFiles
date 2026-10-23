package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Aditya-Nagpal/Cloud-File-Storage-System/services/auth-service/models"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func RegisterUser(ctx context.Context, user models.User) error {

	dob, err := time.Parse("2006-01-02", user.DOB)
	if err != nil {
		return err
	}

	tx, err := DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// 1. Fetch the Free plan from the database.
	var planID int64
	var quotaBytes int64
	err = tx.QueryRow(ctx, `
		SELECT id, storage_limit_bytes
		FROM plans
		WHERE code = 'FREE' AND is_active = TRUE
	`).Scan(&planID, &quotaBytes)
	if err != nil {
		return err
	}

	now := time.Now().UTC()

	// 2. Insert the user and retrieve the generated user ID.
	var userID int64

	err = tx.QueryRow(ctx, `
		INSERT INTO users (
			name,
			email,
			alternate_email,
			contact_number,
			gender,
			dob,
			country,
			timezone,
			about,
			plan,
			password,
			terms_and_privacy,
			created_at,
			updated_at
		)
		VALUES (
			$1, $2, $3, $4, $5, $6, $7,
			$8, $9, $10, $11, $12, $13, $14
		)
		RETURNING id
	`,
		user.Name,
		user.Email,
		user.AlternateEmail,
		user.ContactNumber,
		user.Gender,
		dob,
		user.Country,
		user.Timezone,
		user.About,
		"FREE",
		user.Password,
		user.TermsAndPrivacy,
		now,
		now,
	).Scan(&userID)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "users_email_key" {
			return fmt.Errorf("EMAIL_ALREADY_EXISTS")
		}
		return err
	}

	// 3. Create the user's Free subscription.
	_, err = tx.Exec(ctx, `
		INSERT INTO user_subscriptions (
			user_id,
			plan_id,
			status,
			started_at,
			created_at,
			updated_at
		)
		VALUES ($1, $2, 'ACTIVE', $3, $3, $3)
	`, userID, planID, now)
	if err != nil {
		return err
	}

	// 4. Initialize quota accounting.
	_, err = tx.Exec(ctx, `
		INSERT INTO user_storage (
			user_id,
			quota_bytes,
			used_bytes,
			reserved_bytes,
			updated_at
		)
		VALUES ($1, $2, 0, 0, $3)
	`, userID, quotaBytes, now)
	if err != nil {
		return err
	}

	// 5. Commit all three changes together.
	if err := tx.Commit(ctx); err != nil {
		return err
	}

	return nil
}

func GetUserLoginDetails(ctx context.Context, email string) (*models.UserLogin, error) {
	var userLogin models.UserLogin

	err := DB.QueryRow(ctx, `SELECT id, password FROM users WHERE email=$1`, email).Scan(&userLogin.Id, &userLogin.Password)
	if err == pgx.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	return &userLogin, nil
}

func GetUserHashedPassword(ctx context.Context, email string) (string, error) {
	var hashedPassword string

	err := DB.QueryRow(ctx, `SELECT password FROM users WHERE email=$1`, email).Scan(&hashedPassword)
	if err == pgx.ErrNoRows {
		return "", nil
	} else if err != nil {
		return "", err
	}

	return hashedPassword, nil
}

func UpdateUserPassword(ctx context.Context, email, password string) error {
	_, err := DB.Exec(ctx, `UPDATE users SET password=$1 WHERE email=$2`, password, email)
	if err != nil {
		return err
	}

	return nil
}
