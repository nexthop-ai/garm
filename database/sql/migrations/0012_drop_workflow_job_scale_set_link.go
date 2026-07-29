// Copyright 2026 Cloudbase Solutions SRL
//
//    Licensed under the Apache License, Version 2.0 (the "License"); you may
//    not use this file except in compliance with the License. You may obtain
//    a copy of the License at
//
//         http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
//    WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
//    License for the specific language governing permissions and limitations
//    under the License.

package migrations

import (
	"fmt"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// 0009_scaleset_job_queue linked workflow_jobs rows to the scale set that
// reported them. 0010_scaleset_jobs has since moved scale set jobs into
// their own table, which carries that link itself (scale_set_jobs.
// scale_set_fk_id, refreshed on every job message), so the column, its
// index and its foreign key on workflow_jobs no longer have a reader.
//
// 0009 stays as it is: databases that already applied it need this cleanup,
// and databases that have not still get scale_sets.runner_statistics from it.
func init() {
	Register(&gormigrate.Migration{
		ID: "0012_drop_workflow_job_scale_set_link",
		Migrate: func(tx *gorm.DB) error {
			// Migrations run without a transaction, so a rerun may find the
			// column already gone.
			if !tx.Migrator().HasColumn(&workflowJob0009{}, "scale_set_fk_id") {
				return nil
			}

			// The index goes first so a table rebuild does not try to restore
			// an index on a column that no longer exists.
			if err := tx.Exec("DROP INDEX IF EXISTS " + scaleSetJobIndex).Error; err != nil {
				return fmt.Errorf("dropping %s: %w", scaleSetJobIndex, err)
			}

			drop := func() error {
				// The constraint names the column, so it has to go before the
				// column does.
				if tx.Migrator().HasConstraint(&workflowJob0009{}, scaleSetJobConstraint) {
					if err := tx.Migrator().DropConstraint(&workflowJob0009{}, scaleSetJobConstraint); err != nil {
						return fmt.Errorf("dropping %s: %w", scaleSetJobConstraint, err)
					}
				}
				if err := tx.Migrator().DropColumn(&workflowJob0009{}, "scale_set_fk_id"); err != nil {
					return fmt.Errorf("dropping workflow_jobs.scale_set_fk_id: %w", err)
				}
				return nil
			}

			if tx.Name() != "sqlite" {
				return drop()
			}
			// Both steps rebuild the table on SQLite.
			return sqliteTableRebuild(tx, []string{"workflow_jobs"}, drop)
		},
	})
}
