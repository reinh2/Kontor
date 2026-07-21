package demo

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	HaircutServiceID = "10000000-0000-4000-8000-000000000001"
	BeardServiceID   = "10000000-0000-4000-8000-000000000002"
	ColourServiceID  = "10000000-0000-4000-8000-000000000003"
	KidsServiceID    = "10000000-0000-4000-8000-000000000004"

	AlexStaffID  = "20000000-0000-4000-8000-000000000001"
	NadiaStaffID = "20000000-0000-4000-8000-000000000002"
	TomStaffID   = "20000000-0000-4000-8000-000000000003"
)

type Tenant struct {
	ID       string
	Slug     string
	Name     string
	Timezone string
}

func EnsureFixedTenant(ctx context.Context, pool *pgxpool.Pool, tenant Tenant) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO tenants(id,slug,name,timezone)
		VALUES($1,$2,$3,$4)
		ON CONFLICT(id) DO UPDATE SET
			slug=excluded.slug,name=excluded.name,timezone=excluded.timezone,updated_at=now()`,
		tenant.ID, tenant.Slug, tenant.Name, tenant.Timezone)
	if err != nil {
		return fmt.Errorf("ensure fixed tenant: %w", err)
	}
	return nil
}

// SeedCatalog installs deterministic, repeatable Stage 1 data. It deliberately
// seeds catalog and availability only; conversations and bookings stay real.
func SeedCatalog(ctx context.Context, pool *pgxpool.Pool, tenantID string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin demo seed: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		INSERT INTO services
			(tenant_id,id,slug,name,description,duration_minutes,buffer_before_minutes,buffer_after_minutes,price_minor,currency)
		VALUES
			($1,$2,'haircut','Haircut','A standard appointment',45,0,10,4500,'EUR'),
			($1,$3,'beard-trim','Beard trim','A short appointment',20,0,5,2500,'EUR'),
			($1,$4,'colour','Colour','A longer appointment',90,10,15,9000,'EUR'),
			($1,$5,'kids-cut','Kids cut','A compact appointment',30,0,5,3000,'EUR')
		ON CONFLICT (tenant_id,id) DO UPDATE SET
			name=excluded.name, description=excluded.description,
			duration_minutes=excluded.duration_minutes,
			buffer_before_minutes=excluded.buffer_before_minutes,
			buffer_after_minutes=excluded.buffer_after_minutes,
			price_minor=excluded.price_minor, currency=excluded.currency, active=true`,
		tenantID, HaircutServiceID, BeardServiceID, ColourServiceID, KidsServiceID,
	); err != nil {
		return fmt.Errorf("seed services: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO staff (tenant_id,id,slug,display_name,timezone)
		VALUES
			($1,$2,'alex-rivera','Alex Rivera','Europe/Berlin'),
			($1,$3,'nadia-p','Nadia P.','Europe/Berlin'),
			($1,$4,'tom-b','Tom B.','Europe/Berlin')
		ON CONFLICT (tenant_id,id) DO UPDATE SET
			display_name=excluded.display_name, timezone=excluded.timezone, active=true`,
		tenantID, AlexStaffID, NadiaStaffID, TomStaffID,
	); err != nil {
		return fmt.Errorf("seed staff: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO staff_services (tenant_id,staff_id,service_id)
		VALUES
			($1,$2,$5),($1,$2,$6),($1,$2,$8),
			($1,$3,$5),($1,$3,$7),($1,$3,$8),
			($1,$4,$5),($1,$4,$6)
		ON CONFLICT DO NOTHING`,
		tenantID, AlexStaffID, NadiaStaffID, TomStaffID,
		HaircutServiceID, BeardServiceID, ColourServiceID, KidsServiceID,
	); err != nil {
		return fmt.Errorf("seed staff services: %w", err)
	}

	// Monday-Saturday, 09:00-20:00, with a recurring 13:00-14:00 break.
	for day := 1; day <= 6; day++ {
		for _, staffID := range []string{AlexStaffID, NadiaStaffID, TomStaffID} {
			for _, rule := range []struct {
				kind, start, end string
			}{
				{kind: "working", start: "09:00", end: "20:00"},
				{kind: "break", start: "13:00", end: "14:00"},
			} {
				if _, err := tx.Exec(ctx, `
					INSERT INTO availability_rules
						(tenant_id,staff_id,rule_type,day_of_week,local_start,local_end)
					SELECT $1,$2,$3,$4,$5::time,$6::time
					WHERE NOT EXISTS (
						SELECT 1 FROM availability_rules
						WHERE tenant_id=$1 AND staff_id=$2 AND rule_type=$3
						  AND day_of_week=$4 AND local_start=$5::time AND local_end=$6::time
					)`, tenantID, staffID, rule.kind, day, rule.start, rule.end); err != nil {
					return fmt.Errorf("seed availability: %w", err)
				}
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit demo seed: %w", err)
	}
	return nil
}
