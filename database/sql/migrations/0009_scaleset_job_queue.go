// Copyright 2026 Cloudbase Solutions SRL
//
//	Licensed under the Apache License, Version 2.0 (the "License"); you may
//	not use this file except in compliance with the License. You may obtain
//	a copy of the License at
//
//	     http://www.apache.org/licenses/LICENSE-2.0
//
//	Unless required by applicable law or agreed to in writing, software
//	distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
//	WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
//	License for the specific language governing permissions and limitations
//	under the License.

package migrations

import (
	"errors"
	"fmt"
	"strings"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// scaleSet0009 adds the cached RunnerScaleSetStatistic that the scale set
// listener receives from GitHub on every message session response. Stub:
// AutoMigrate only reconciles the fields declared here.
type scaleSet0009 struct {
	ID               uint `gorm:"primarykey"`
	RunnerStatistics datatypes.JSON
}

func (scaleSet0009) TableName() string { return "scale_sets" }

// workflowJob0009 links a job to the GARM scale set whose listener
// reported it, so the queue view can attribute jobs per scale set. The
// link is cleared, not the job removed, when the scale set goes away.
type workflowJob0009 struct {
	ID           int64        `gorm:"index"`
	ScaleSetFkID *uint        `gorm:"index"`
	ScaleSet     scaleSet0009 `gorm:"foreignKey:ScaleSetFkID;constraint:OnDelete:SET NULL"`
}

func (workflowJob0009) TableName() string { return "workflow_jobs" }

const (
	scaleSetJobConstraint = "fk_workflow_jobs_scale_set"
	scaleSetJobIndex      = "idx_workflow_jobs_scale_set_fk_id"
)

// hasScaleSetJobConstraint reports whether workflow_jobs already carries
// the scale set foreign key with the delete action this migration wants.
// Databases that grew the column through AutoMigrate before it was
// covered by a migration have the constraint without a delete action.
func hasScaleSetJobConstraint(tx *gorm.DB) (present, upToDate bool, err error) {
	if !tx.Migrator().HasConstraint(&workflowJob0009{}, scaleSetJobConstraint) {
		return false, false, nil
	}
	if tx.Name() != "sqlite" {
		// Trust the constraint on engines where ADD CONSTRAINT is cheap and
		// in place; a mismatch there is corrected by the same drop/create.
		var action string
		err := tx.Raw(`SELECT delete_rule FROM information_schema.referential_constraints WHERE constraint_name = ?`, scaleSetJobConstraint).Scan(&action).Error
		return true, err == nil && strings.EqualFold(action, "SET NULL"), err
	}
	var ddl string
	if err := tx.Raw(`SELECT sql FROM sqlite_master WHERE type='table' AND name='workflow_jobs'`).Scan(&ddl).Error; err != nil {
		return true, false, fmt.Errorf("reading workflow_jobs DDL: %w", err)
	}
	idx := strings.Index(ddl, "`"+scaleSetJobConstraint+"`")
	if idx < 0 {
		return true, false, nil
	}
	clause := ddl[idx:]
	if end := strings.Index(clause, ",\n"); end > 0 {
		clause = clause[:end]
	} else if end := strings.Index(clause, ",CONSTRAINT"); end > 0 {
		clause = clause[:end]
	}
	return true, strings.Contains(clause, "ON DELETE SET NULL"), nil
}

func init() {
	Register(&gormigrate.Migration{
		ID: "0009_scaleset_job_queue",
		Migrate: func(tx *gorm.DB) error {
			// Columns and the index first; these are plain ALTER TABLE ADD
			// COLUMN on every engine.
			if !tx.Migrator().HasColumn(&scaleSet0009{}, "RunnerStatistics") {
				if err := tx.Migrator().AddColumn(&scaleSet0009{}, "RunnerStatistics"); err != nil {
					return fmt.Errorf("adding scale_sets.runner_statistics: %w", err)
				}
			}
			if !tx.Migrator().HasColumn(&workflowJob0009{}, "ScaleSetFkID") {
				if err := tx.Migrator().AddColumn(&workflowJob0009{}, "ScaleSetFkID"); err != nil {
					return fmt.Errorf("adding workflow_jobs.scale_set_fk_id: %w", err)
				}
			}
			if !tx.Migrator().HasIndex(&workflowJob0009{}, scaleSetJobIndex) {
				if err := tx.Migrator().CreateIndex(&workflowJob0009{}, scaleSetJobIndex); err != nil {
					return fmt.Errorf("creating %s: %w", scaleSetJobIndex, err)
				}
			}

			present, upToDate, err := hasScaleSetJobConstraint(tx)
			if err != nil {
				return err
			}
			if upToDate {
				return nil
			}

			// Rows pointing at a scale set that no longer exists would fail
			// the constraint; clear them the way ON DELETE SET NULL would.
			if err := tx.Exec("UPDATE workflow_jobs SET scale_set_fk_id = NULL WHERE scale_set_fk_id IS NOT NULL AND scale_set_fk_id NOT IN (SELECT id FROM scale_sets)").Error; err != nil {
				return fmt.Errorf("clearing dangling scale set references: %w", err)
			}

			applyConstraint := func() error {
				if present {
					if err := tx.Migrator().DropConstraint(&workflowJob0009{}, scaleSetJobConstraint); err != nil {
						return fmt.Errorf("dropping %s: %w", scaleSetJobConstraint, err)
					}
				}
				if err := tx.Migrator().CreateConstraint(&workflowJob0009{}, scaleSetJobConstraint); err != nil {
					return fmt.Errorf("creating %s: %w", scaleSetJobConstraint, err)
				}
				return nil
			}

			if tx.Name() != "sqlite" {
				return applyConstraint()
			}

			// SQLite rebuilds the table to change constraints (create temp,
			// copy, drop, rename). Dropping a table with foreign keys enforced
			// fails when rows reference or are referenced, and the rebuild
			// loses the table's standalone indexes, so follow SQLite's ALTER
			// procedure: constraints off, rebuild, restore indexes. Same
			// approach as 0008.
			var indexes []struct {
				Name string
				SQL  string
			}
			if err := tx.Raw(`SELECT name, sql FROM sqlite_master WHERE type='index' AND sql IS NOT NULL AND tbl_name = 'workflow_jobs'`).Scan(&indexes).Error; err != nil {
				return fmt.Errorf("capturing workflow_jobs indexes: %w", err)
			}
			if err := tx.Exec("PRAGMA foreign_keys = OFF").Error; err != nil {
				return fmt.Errorf("disabling foreign keys: %w", err)
			}
			migrateErr := applyConstraint()
			if err := tx.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
				return errors.Join(migrateErr, fmt.Errorf("re-enabling foreign keys: %w", err))
			}
			if migrateErr != nil {
				return migrateErr
			}
			for _, index := range indexes {
				var count int64
				if err := tx.Raw("SELECT count(*) FROM sqlite_master WHERE type='index' AND name = ?", index.Name).Scan(&count).Error; err != nil {
					return fmt.Errorf("checking index %s: %w", index.Name, err)
				}
				if count == 0 {
					if err := tx.Exec(index.SQL).Error; err != nil {
						return fmt.Errorf("restoring index %s: %w", index.Name, err)
					}
				}
			}
			var violations int64
			if err := tx.Raw("SELECT count(*) FROM pragma_foreign_key_check('workflow_jobs')").Scan(&violations).Error; err != nil {
				return fmt.Errorf("checking foreign keys: %w", err)
			}
			if violations > 0 {
				return fmt.Errorf("%d workflow_jobs rows violate foreign keys after migration", violations)
			}
			return nil
		},
	})
}
