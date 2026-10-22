package seed

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/KayaOrhan/SmartCampaignComposer/internal/segment"
)

const Population = 5000

type User struct {
	ID                                                int64
	FirstName, LastName, Email, Phone, City, Language string
	CreatedAt                                         time.Time
	CartDays, OrderDays                               int
	SMS, EmailConsent                                 *bool
}

func Generate(asOf time.Time) []User {
	rng := rand.New(rand.NewSource(2026))
	cities := []string{"Istanbul", "İstanbul", "ISTANBUL", "istanbul", "Ankara", "İzmir", "Bursa"}
	firstNames := []string{"Deniz", "Çağla", "İpek", "Ali", "Abdülkadir"}
	users := make([]User, 0, Population)
	for i := 1; i <= Population; i++ {
		u := User{ID: int64(i), FirstName: firstNames[rng.Intn(len(firstNames))], LastName: fmt.Sprintf("Example%d", i), Email: fmt.Sprintf("user%05d@example.test", i), Phone: fmt.Sprintf("mock-phone-%06d", i), City: cities[rng.Intn(len(cities))], Language: []string{"tr", "en"}[rng.Intn(2)], CreatedAt: asOf.UTC().Add(-400 * 24 * time.Hour), CartDays: rng.Intn(70), OrderDays: rng.Intn(90)}
		if i%7 != 0 {
			granted := i%3 != 0
			u.SMS = &granted
		}
		if i%11 != 0 {
			granted := i%4 != 0
			u.EmailConsent = &granted
		}
		users = append(users, u)
	}
	return users
}

// Ensure only inserts the first seed version; it never overwrites consent edits.
// ponytail: a single API instance seeds once; coordinate before scaling API replicas.
func Ensure(ctx context.Context, db *sql.DB, asOf time.Time) (result error) {
	if asOf.IsZero() {
		return errors.New("seed reference time required")
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return errors.New("begin seed failed")
	}
	defer func() {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			result = errors.Join(result, errors.New("rollback seed failed"))
		}
	}()
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app.seed_runs WHERE version=1)`).Scan(&exists); err != nil {
		return errors.New("check seed version failed")
	}
	if exists {
		if err := tx.Commit(); err != nil {
			return errors.New("finish seed check failed")
		}
		return nil
	}
	for _, u := range Generate(asOf) {
		if _, err := tx.ExecContext(ctx, `INSERT INTO app.users(id,first_name,last_name,email,phone,city,city_norm,language,created_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, u.ID, u.FirstName, u.LastName, u.Email, u.Phone, u.City, segment.NormalizeCity(u.City), u.Language, u.CreatedAt); err != nil {
			return errors.New("insert seed user failed")
		}
		for channel, granted := range map[string]*bool{"sms": u.SMS, "email": u.EmailConsent} {
			if granted == nil {
				continue
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO app.consents(user_id,channel,granted,updated_at) VALUES ($1,$2,$3,$4)`, u.ID, channel, *granted, asOf.UTC()); err != nil {
				return errors.New("insert seed consent failed")
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO app.cart_events(user_id,product_id,added_at) VALUES ($1,$2,$3),($1,$2,$3)`, u.ID, u.ID%100+1, asOf.UTC().Add(-time.Duration(u.CartDays)*24*time.Hour-time.Hour)); err != nil {
			return errors.New("insert seed cart failed")
		}
		if u.ID%5 != 0 {
			if _, err := tx.ExecContext(ctx, `INSERT INTO app.orders(user_id,placed_at,total) VALUES ($1,$2,$3)`, u.ID, asOf.UTC().Add(-time.Duration(u.OrderDays)*24*time.Hour-time.Hour), 100); err != nil {
				return errors.New("insert seed order failed")
			}
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO app.seed_runs(version,reference_time) VALUES (1,$1)`, asOf.UTC()); err != nil {
		return errors.New("record seed version failed")
	}
	if err := tx.Commit(); err != nil {
		return errors.New("commit seed failed")
	}
	return nil
}
