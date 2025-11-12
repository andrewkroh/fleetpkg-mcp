// Licensed to Elasticsearch B.V. under one or more agreements.
// Elasticsearch B.V. licenses this file to you under the Apache 2.0 License.
// See the LICENSE file in the project root for more information.

package fleetsql

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/andrewkroh/go-ecs"
	"github.com/andrewkroh/go-fleetpkg"

	"github.com/andrewkroh/fleetpkg-mcp/internal/database"
)

// TableSchemas returns a slice of SQL table creation statements.
// The statements include comments explaining the table's purpose and
// details about each column.
func TableSchemas() []string {
	return database.Creates[:]
}

// WritePackages writes integration packages into the database.
// It creates the necessary tables and inserts each package in a transaction.
// Returns an error if table creation or package insertion fails.
func WritePackages(ctx context.Context, db *sql.DB, pkgs []fleetpkg.Integration) error {
	// Create tables (assumes they do not exist).
	if err := createTables(ctx, db); err != nil {
		return fmt.Errorf("failed creating tables: %w", err)
	}

	// Write each package to DB in a TX.
	for _, in := range pkgs {
		if err := insertPackage(ctx, db, &in); err != nil {
			return fmt.Errorf("failed inserting %q: %w", filepath.Base(in.Path()), err)
		}
	}

	return nil
}

// createTables creates the database tables if they do not exist.
func createTables(ctx context.Context, db *sql.DB) (err error) {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer txDone(tx, &err)

	for _, t := range database.Creates {
		if _, err := tx.ExecContext(ctx, t); err != nil {
			return fmt.Errorf("failed creating table: %q: %w", t, err)
		}
	}
	return nil
}

func insertPackage(ctx context.Context, db *sql.DB, in *fleetpkg.Integration) (err error) {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer txDone(tx, &err)

	q := database.New(tx)
	integID, err := insertManifest(ctx, q, in)
	if err != nil {
		return err
	}

	// Integration categories.
	for _, cat := range in.Manifest.Categories {
		err = q.InsertIntegrationCategory(ctx, database.InsertIntegrationCategoryParams{
			IntegrationID: integID,
			Category:      cat,
		})
		if err != nil {
			return err
		}
	}

	// Integration icons.
	for _, icon := range in.Manifest.Icons {
		_, err = q.InsertIntegrationIcon(ctx, database.InsertIntegrationIconParams{
			IntegrationID: integID,
			Src:           sqlStringEmtpyIsNull(icon.Src),
			Title:         sqlStringEmtpyIsNull(icon.Title),
			Size:          sqlStringEmtpyIsNull(icon.Size),
			Type:          sqlStringEmtpyIsNull(icon.Type),
			DarkMode:      sqlNullBool(icon.DarkMode),
		})
		if err != nil {
			return err
		}
	}

	// Integration screenshots.
	for _, screenshot := range in.Manifest.Screenshots {
		_, err = q.InsertIntegrationScreenshot(ctx, database.InsertIntegrationScreenshotParams{
			IntegrationID: integID,
			Src:           sqlStringEmtpyIsNull(screenshot.Src),
			Title:         sqlStringEmtpyIsNull(screenshot.Title),
			Size:          sqlStringEmtpyIsNull(screenshot.Size),
			Type:          sqlStringEmtpyIsNull(screenshot.Type),
		})
		if err != nil {
			return err
		}
	}

	// Discovery fields.
	if in.Manifest.Discovery != nil {
		for _, field := range in.Manifest.Discovery.Fields {
			_, err = q.InsertDiscoveryField(ctx, database.InsertDiscoveryFieldParams{
				IntegrationID: integID,
				Name:          sqlStringEmtpyIsNull(field.Name),
			})
			if err != nil {
				return err
			}
		}
	}

	// Build manifest.
	if in.Build != nil {
		_, err = q.InsertBuildManifest(ctx, database.InsertBuildManifestParams{
			IntegrationID:                 integID,
			DependenciesEcsReference:      sqlStringEmtpyIsNull(in.Build.Dependencies.ECS.Reference),
			DependenciesEcsImportMappings: sqlNullBool(in.Build.Dependencies.ECS.ImportMappings),
			FilePath:                      in.Build.Path(),
		})
		if err != nil {
			return err
		}
	}

	// Integration top-level variables.
	for _, v := range in.Manifest.Vars {
		varID, err := insertVar(ctx, q, &v)
		if err != nil {
			return err
		}

		err = q.InsertIntegrationVar(ctx, database.InsertIntegrationVarParams{
			IntegrationID: integID,
			VarID:         varID,
		})
		if err != nil {
			return err
		}
	}

	// Policy templates.
	for _, pt := range in.Manifest.PolicyTemplates {
		ptID, err := insertPolicyTemplate(ctx, q, integID, &pt)
		if err != nil {
			return err
		}

		// Policy template categories.
		for _, cat := range pt.Categories {
			err = q.InsertPolicyTemplateCategory(ctx, database.InsertPolicyTemplateCategoryParams{
				PolicyTemplateID: ptID,
				Category:         cat,
			})
			if err != nil {
				return err
			}
		}

		// Policy template data streams.
		for _, dsName := range pt.DataStreams {
			err = q.InsertPolicyTemplateDataStream(ctx, database.InsertPolicyTemplateDataStreamParams{
				PolicyTemplateID: ptID,
				DataStreamName:   dsName,
			})
			if err != nil {
				return err
			}
		}

		// Policy template icons.
		for _, icon := range pt.Icons {
			_, err = q.InsertPolicyTemplateIcon(ctx, database.InsertPolicyTemplateIconParams{
				PolicyTemplateID: ptID,
				Src:              sqlStringEmtpyIsNull(icon.Src),
				Title:            sqlStringEmtpyIsNull(icon.Title),
				Size:             sqlStringEmtpyIsNull(icon.Size),
				Type:             sqlStringEmtpyIsNull(icon.Type),
				DarkMode:         sqlNullBool(icon.DarkMode),
			})
			if err != nil {
				return err
			}
		}

		// Policy template screenshots.
		for _, screenshot := range pt.Screenshots {
			_, err = q.InsertPolicyTemplateScreenshot(ctx, database.InsertPolicyTemplateScreenshotParams{
				PolicyTemplateID: ptID,
				Src:              sqlStringEmtpyIsNull(screenshot.Src),
				Title:            sqlStringEmtpyIsNull(screenshot.Title),
				Size:             sqlStringEmtpyIsNull(screenshot.Size),
				Type:             sqlStringEmtpyIsNull(screenshot.Type),
			})
			if err != nil {
				return err
			}
		}

		// Policy template variables.
		for _, v := range pt.Vars {
			varID, err := insertVar(ctx, q, &v)
			if err != nil {
				return err
			}

			err = q.InsertPolicyTemplateVar(ctx, database.InsertPolicyTemplateVarParams{
				PolicyTemplateID: ptID,
				VarID:            varID,
			})
			if err != nil {
				return err
			}
		}

		// Policy template inputs.
		for _, input := range pt.Inputs {
			ptInputID, err := q.InsertPolicyTemplateInput(ctx, database.InsertPolicyTemplateInputParams{
				PolicyTemplateID: ptID,
				Type:             input.Type,
				Title:            input.Title,
				Description:      input.Description,
				InputGroup:       sqlStringEmtpyIsNull(input.InputGroup),
				TemplatePath:     sqlStringEmtpyIsNull(input.TemplatePath),
				Multi:            sqlNullBool(input.Multi),
			})
			if err != nil {
				return err
			}

			// Policy template input variables.
			for _, v := range input.Vars {
				varID, err := insertVar(ctx, q, &v)
				if err != nil {
					return err
				}

				err = q.InsertPolicyTemplateInputVars(ctx, database.InsertPolicyTemplateInputVarsParams{
					PolicyTemplateInputID: ptInputID,
					VarID:                 varID,
				})
				if err != nil {
					return err
				}
			}
		}
	}

	// Data streams.
	for _, ds := range in.DataStreams {
		dsID, err := insertDataStream(ctx, q, integID, ds)
		if err != nil {
			return err
		}

		// Data stream streams (aka inputs).
		for _, s := range ds.Manifest.Streams {
			sID, err := insertStream(ctx, q, dsID, &s)
			if err != nil {
				return err
			}

			// Data stream vars.
			for _, v := range s.Vars {
				varID, err := insertVar(ctx, q, &v)
				if err != nil {
					return err
				}

				err = q.InsertStreamVar(ctx, database.InsertStreamVarParams{
					StreamID: sID,
					VarID:    varID,
				})
				if err != nil {
					return err
				}
			}

			// Data stream fields.
			flat, err := fleetpkg.FlattenFields(ds.AllFields())
			if err != nil {
				return err
			}
			for _, f := range flat {
				var externalDef *ecs.Field
				if f.External == "ecs" && in.Build != nil && in.Build.Dependencies.ECS.Reference != "" {
					externalDef, _ = ecs.Lookup(f.Name, strings.TrimPrefix(in.Build.Dependencies.ECS.Reference, "git@"))
				}

				fieldID, err := insertField(ctx, q, &f, externalDef)
				if err != nil {
					return err
				}

				err = q.InsertDataStreamField(ctx, database.InsertDataStreamFieldParams{
					DataStreamID:   dsID,
					FieldID:        fieldID,
					FieldsFileName: filepath.Base(f.FileMetadata.Path()),
				})
				if err != nil {
					return err
				}
			}
		}

		// Data stream ingest pipelines.
		for name, pipeline := range ds.Pipelines {
			pipelineID, err := q.InsertIngestPipeline(ctx, database.InsertIngestPipelineParams{
				DataStreamID: dsID,
				Name:         sqlStringEmtpyIsNull(name),
				Description:  sqlStringEmtpyIsNull(pipeline.Description),
				Version:      sqlNullInt64(pipeline.Version),
				Meta:         jsonNullString(pipeline.Meta),
				FilePath:     pipeline.Path(),
			})
			if err != nil {
				return err
			}

			// Flatten and insert processors.
			processors, err := FlattenProcessors(pipeline.Processors, "/processors")
			if err != nil {
				return fmt.Errorf("failed to flatten processors for pipeline %s: %w", name, err)
			}
			for _, proc := range processors {
				attrs, err := proc.MarshalAttributes()
				if err != nil {
					return fmt.Errorf("failed to marshal processor attributes: %w", err)
				}

				_, err = q.InsertIngestProcessor(ctx, database.InsertIngestProcessorParams{
					IngestPipelineID: pipelineID,
					Type:             proc.Type,
					Attributes:       sqlStringEmtpyIsNull(attrs),
					JsonPointer:      proc.JSONPointer,
					FilePath:         proc.FilePath,
					LineNumber:       int64(proc.Line),
					Col:              int64(proc.Column),
				})
				if err != nil {
					return fmt.Errorf("failed to insert processor %s at %s: %w", proc.Type, proc.JSONPointer, err)
				}
			}

			// Flatten and insert global on_failure processors.
			if len(pipeline.OnFailure) > 0 {
				onFailureProcessors, err := FlattenProcessors(pipeline.OnFailure, "/on_failure")
				if err != nil {
					return fmt.Errorf("failed to flatten on_failure processors for pipeline %s: %w", name, err)
				}
				for _, proc := range onFailureProcessors {
					attrs, err := proc.MarshalAttributes()
					if err != nil {
						return fmt.Errorf("failed to marshal on_failure processor attributes: %w", err)
					}

					_, err = q.InsertIngestProcessor(ctx, database.InsertIngestProcessorParams{
						IngestPipelineID: pipelineID,
						Type:             proc.Type,
						Attributes:       sqlStringEmtpyIsNull(attrs),
						JsonPointer:      proc.JSONPointer,
						FilePath:         proc.FilePath,
						LineNumber:       int64(proc.Line),
						Col:              int64(proc.Column),
					})
					if err != nil {
						return fmt.Errorf("failed to insert on_failure processor %s at %s: %w", proc.Type, proc.JSONPointer, err)
					}
				}
			}
		}

		// Data stream sample event.
		if ds.SampleEvent != nil {
			_, err = q.InsertSampleEvent(ctx, database.InsertSampleEventParams{
				DataStreamID: dsID,
				Event:        jsonNullString(ds.SampleEvent.Event),
				FilePath:     ds.SampleEvent.Path(),
			})
			if err != nil {
				return err
			}
		}
	}

	// Integration transforms.
	for _, t := range in.Transforms {
		transformID, err := insertTransform(ctx, q, integID, t)
		if err != nil {
			return err
		}

		// Transform fields.
		flat, err := fleetpkg.FlattenFields(t.Fields)
		if err != nil {
			return err
		}
		for _, f := range flat {
			var externalDef *ecs.Field
			if f.External == "ecs" && in.Build != nil && in.Build.Dependencies.ECS.Reference != "" {
				externalDef, _ = ecs.Lookup(f.Name, strings.TrimPrefix(in.Build.Dependencies.ECS.Reference, "git@"))
			}

			fieldID, err := insertField(ctx, q, &f, externalDef)
			if err != nil {
				return err
			}

			err = q.InsertTransformField(ctx, database.InsertTransformFieldParams{
				TransformID: transformID,
				FieldID:     fieldID,
			})
			if err != nil {
				return err
			}
		}
	}

	// Integration changelog.
	if in.Changelog.Path() != "" {
		changelogID, err := q.InsertChangelog(ctx, database.InsertChangelogParams{
			IntegrationID: integID,
			FilePath:      in.Changelog.Path(),
		})
		if err != nil {
			return err
		}

		// Changelog releases.
		for _, release := range in.Changelog.Releases {
			releaseID, err := q.InsertRelease(ctx, database.InsertReleaseParams{
				ChangelogID: changelogID,
				Version:     sqlStringEmtpyIsNull(release.Version),
				FilePath:    release.Path(),
				LineNumber:  sql.NullInt64{Int64: int64(release.Line()), Valid: release.Line() > 0},
				Col:         sql.NullInt64{Int64: int64(release.Column()), Valid: release.Column() > 0},
			})
			if err != nil {
				return err
			}

			// Release changes.
			for _, change := range release.Changes {
				_, err = q.InsertChange(ctx, database.InsertChangeParams{
					ReleaseID:   releaseID,
					Description: sqlStringEmtpyIsNull(change.Description),
					Type:        sqlStringEmtpyIsNull(change.Type),
					Link:        sqlStringEmtpyIsNull(change.Link),
					FilePath:    change.Path(),
					LineNumber:  sql.NullInt64{Int64: int64(change.Line()), Valid: change.Line() > 0},
					Col:         sql.NullInt64{Int64: int64(change.Column()), Valid: change.Column() > 0},
				})
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func insertManifest(ctx context.Context, q *database.Queries, in *fleetpkg.Integration) (int64, error) {
	m := in.Manifest
	p := database.InsertIntegrationParams{
		Name:                          m.Name,
		FilePath:                      m.Path(),
		DirName:                       filepath.Base(filepath.Dir(m.Path())),
		Title:                         m.Title,
		Version:                       m.Version,
		Description:                   m.Description,
		Type:                          m.Type,
		FormatVersion:                 m.FormatVersion,
		License:                       sqlStringEmtpyIsNull(m.License),
		Release:                       sqlStringEmtpyIsNull(m.Release),
		PolicyTemplatesBehavior:       sqlStringEmtpyIsNull(m.PolicyTemplatesBehavior),
		ConditionsElasticSubscription: sqlStringEmtpyIsNull(m.Conditions.Elastic.Subscription),
		ConditionsKibanaVersion:       sqlStringEmtpyIsNull(m.Conditions.Kibana.Version),
		SourceLicense:                 sqlStringEmtpyIsNull(m.Source.License),
		OwnerGithub:                   m.Owner.Github,
		OwnerType:                     m.Owner.Type,
	}
	if m.Agent != nil {
		p.AgentPrivilegesRoot = sql.NullBool{Bool: m.Agent.Privileges.Root, Valid: true}
	}
	if m.Elasticsearch != nil {
		p.ElasticsearchPrivilegesCluster = jsonNullString(m.Elasticsearch.Privileges.Cluster)
	}
	id, err := q.InsertIntegration(ctx, p)
	if err != nil {
		return 0, err
	}
	return id, nil
}

func insertPolicyTemplate(ctx context.Context, q *database.Queries, integID int64, pt *fleetpkg.PolicyTemplate) (int64, error) {
	p := database.InsertPolicyTemplateParams{
		IntegrationID: integID,
		Name:          pt.Name,
		Title:         pt.Title,
		Description:   pt.Description,
		Type:          sqlNullString(&pt.Type),
	}
	if pt.DeploymentModes != nil {
		p.DeploymentModesDefaultEnabled = sqlNullBool(pt.DeploymentModes.Default.Enabled)
		p.DeploymentModesAgentlessEnabled = sqlNullBool(pt.DeploymentModes.Agentless.Enabled)
		p.DeploymentModesAgentlessIsDefault = sqlNullBool(pt.DeploymentModes.Agentless.IsDefault)
		p.DeploymentModesAgentlessOrganization = sqlStringEmtpyIsNull(pt.DeploymentModes.Agentless.Organization)
		p.DeploymentModesAgentlessDivision = sqlStringEmtpyIsNull(pt.DeploymentModes.Agentless.Division)
		p.DeploymentModesAgentlessTeam = sqlStringEmtpyIsNull(pt.DeploymentModes.Agentless.Team)
		if pt.DeploymentModes.Agentless.Resources != nil {
			p.DeploymentModesAgentlessResourcesRequestsMemory = sqlStringEmtpyIsNull(pt.DeploymentModes.Agentless.Resources.Requests.Memory)
			p.DeploymentModesAgentlessResourcesRequestsCpu = sqlStringEmtpyIsNull(pt.DeploymentModes.Agentless.Resources.Requests.CPU)
		}
	}
	id, err := q.InsertPolicyTemplate(ctx, p)
	if err != nil {
		return 0, err
	}
	return id, nil
}

func insertDataStream(ctx context.Context, q *database.Queries, integID int64, ds *fleetpkg.DataStream) (int64, error) {
	m := ds.Manifest
	p := database.InsertDataStreamParams{
		IntegrationID:   integID,
		Name:            filepath.Base(ds.Path()),
		FilePath:        ds.Path(),
		Title:           m.Title,
		Dataset:         sqlStringEmtpyIsNull(m.Dataset),
		DatasetIsPrefix: sqlNullBool(m.DatasetIsPrefix),
		IlmPolicy:       sqlStringEmtpyIsNull(m.ILMPolicy),
		Release:         sqlStringEmtpyIsNull(m.Release),
		Type:            sqlStringEmtpyIsNull(m.Type),
	}
	if m.Elasticsearch != nil {
		es := m.Elasticsearch
		p.ElasticsearchIndexMode = sqlStringEmtpyIsNull(es.IndexMode)
		p.ElasticsearchSourceMode = sqlStringEmtpyIsNull(es.IndexMode)
		p.ElasticsearchDynamicDataset = sqlNullBool(es.DynamicDataset)
		p.ElasticsearchDynamicNamespace = sqlNullBool(es.DynamicNamespace)
		if es.Privileges != nil {
			p.ElasticsearchPrivilegesProperties = jsonNullString(es.Privileges.Properties)
		}
		if es.IndexTemplate != nil {
			p.ElasticsearchIndexTemplateSettings = jsonNullString(es.IndexTemplate.Settings)
			p.ElasticsearchIndexTemplateMappings = jsonNullString(es.IndexTemplate.Mappings)
			if es.IndexTemplate.IngestPipeline != nil {
				p.ElasticsearchIndexTemplateIngestPipelineName = sqlStringEmtpyIsNull(es.IndexTemplate.IngestPipeline.Name)
			}
			if es.IndexTemplate.DataStream != nil {
				p.ElasticsearchIndexTemplateDataStreamHidden = sqlNullBool(es.IndexTemplate.DataStream.Hidden)
			}
		}
	}
	dsID, err := q.InsertDataStream(ctx, p)
	if err != nil {
		return 0, err
	}
	return dsID, nil
}

func insertStream(ctx context.Context, q *database.Queries, dsID int64, s *fleetpkg.Stream) (int64, error) {
	p := database.InsertStreamParams{
		DataStreamID: dsID,
		Input:        s.Input,
		Description:  s.Description,
		Title:        s.Title,
		TemplatePath: sqlStringEmtpyIsNull(s.TemplatePath),
		Enabled:      sqlNullBool(s.Enabled),
	}
	id, err := q.InsertStream(ctx, p)
	if err != nil {
		return 0, err
	}
	return id, nil
}

func insertVar(ctx context.Context, q *database.Queries, v *fleetpkg.Var) (int64, error) {
	id, err := q.InsertVar(ctx, database.InsertVarParams{
		Name:                  v.Name,
		DefaultValue:          jsonNullString(v.Default),
		Description:           sql.NullString{String: v.Description, Valid: true},
		Type:                  v.Type,
		Title:                 sql.NullString{String: v.Title, Valid: true},
		Multi:                 sqlNullBool(v.Multi),
		Required:              sqlNullBool(v.Required),
		Secret:                sqlNullBool(v.Secret),
		ShowUser:              sqlNullBool(v.ShowUser),
		HideInDeploymentModes: jsonNullString(v.HideInDeploymentModes),
		FilePath:              v.Path(),
		LineNumber:            int64(v.Line()),
		Col:                   int64(v.Column()),
	})
	if err != nil {
		return 0, err
	}

	// Var options.
	for _, opt := range v.Options {
		_, err = q.InsertVarOption(ctx, database.InsertVarOptionParams{
			VarID: id,
			Value: sqlStringEmtpyIsNull(opt.Value),
			Text:  sqlStringEmtpyIsNull(opt.Text),
		})
		if err != nil {
			return 0, err
		}
	}

	return id, nil
}

func insertField(ctx context.Context, q *database.Queries, f *fleetpkg.Field, externalDef *ecs.Field) (int64, error) {
	p := database.InsertFieldParams{
		Name:            f.Name,
		Type:            sqlStringEmtpyIsNull(f.Type),
		Description:     sqlStringEmtpyIsNull(f.Description),
		Value:           jsonNullString(f.Value),
		Example:         jsonNullString(f.Example),
		Pattern:         sqlStringEmtpyIsNull(f.Pattern),
		DateFormat:      sqlStringEmtpyIsNull(f.DateFormat),
		Analyzer:        sqlStringEmtpyIsNull(f.Analyzer),
		SearchAnalyzer:  sqlStringEmtpyIsNull(f.SearchAnalyzer),
		IgnoreAbove:     sql.NullInt64{Int64: int64(f.IgnoreAbove), Valid: f.IgnoreAbove > 0},
		MultiFields:     jsonNullString(f.MultiFields),
		Enabled:         sqlNullBool(f.Enabled),
		Dynamic:         sqlStringEmtpyIsNull(f.Dynamic),
		Indexed:         sqlNullBool(f.Index),
		DocValues:       sqlNullBool(f.DocValues),
		CopyTo:          sqlStringEmtpyIsNull(f.CopyTo),
		ScalingFactor:   sqlNullInt64(f.ScalingFactor),
		AliasTargetPath: sqlStringEmtpyIsNull(f.AliasTargetPath),
		Normalize:       jsonNullString(f.Normalize),
		Normalizer:      sqlStringEmtpyIsNull(f.Normalizer),
		NullValue:       jsonNullString(f.NullValue),
		Dimension:       sqlNullBool(f.Dimension),
		MetricType:      sqlStringEmtpyIsNull(f.MetricType),
		External:        sqlStringEmtpyIsNull(f.External),
		YamlPath:        sqlStringEmtpyIsNull(f.YAMLPath),
		FilePath:        f.Path(),
		LineNumber:      int64(f.Line()),
		Col:             int64(f.Column()),
	}
	// Merge in 'external: ecs' properties.
	if externalDef != nil {
		if !p.Type.Valid && externalDef.DataType != "" {
			p.Type = sqlStringEmtpyIsNull(externalDef.DataType)
		}
		if !p.Pattern.Valid && externalDef.Pattern != "" {
			p.Pattern = sqlStringEmtpyIsNull(externalDef.Pattern)
		}
		if !p.Normalizer.Valid && externalDef.Array {
			p.Normalize = jsonNullString([]string{"array"})
		}
		if !p.Description.Valid && externalDef.Description != "" {
			p.Description = sqlStringEmtpyIsNull(externalDef.Description)
		}
	} else if externalDef == nil && f.External == "ecs" {
		p.Unresolvable = sql.NullInt64{Int64: 1, Valid: true}
	}
	id, err := q.InsertField(ctx, p)
	if err != nil {
		return 0, err
	}
	return id, nil
}

func insertTransform(ctx context.Context, q *database.Queries, integID int64, t *fleetpkg.Transform) (int64, error) {
	p := database.InsertTransformParams{
		IntegrationID: integID,
		Name:          filepath.Base(t.Path()),
		FilePath:      t.Path(),
	}
	if t.Transform != nil {
		tr := t.Transform

		// Source fields
		if tr.Source != nil {
			// TODO: Index needs encoded to JSON and stored as a string.
			if index, ok := tr.Source.Index.(string); ok {
				p.TransformSourceIndex = index
			} else if indices, ok := tr.Source.Index.([]string); ok && len(indices) > 0 {
				p.TransformSourceIndex = indices[0] // Use the first index
			}

			p.TransformSourceQuery = jsonNullString(tr.Source.Query)
			p.TransformSourceRuntimeMappings = jsonNullString(tr.Source.RuntimeMappings)
		}

		// Destination fields
		if tr.Dest != nil {
			if tr.Dest.Index != nil {
				// go-fleetpkg should make this a string b/c it is required.
				p.TransformDestIndex = *tr.Dest.Index
			}
			p.TransformDestPipeline = sqlNullString(tr.Dest.Pipeline)
		}

		// Pivot fields
		if tr.Pivot != nil {
			p.TransformPivotGroupBy = jsonNullString(tr.Pivot.GroupBy)
			p.TransformPivotAggregations = jsonNullString(tr.Pivot.Aggregations)
			p.TransformPivotAggs = jsonNullString(tr.Pivot.Aggs)
		}

		// Latest fields
		if tr.Latest != nil {
			p.TransformLatestSort = sqlNullString(tr.Latest.Sort)
			p.TransformLatestUniqueKey = jsonNullString(tr.Latest.UniqueKey)
		}

		// Description and frequency
		p.TransformDescription = sqlNullString(tr.Description)
		p.TransformFrequency = sqlNullString(tr.Frequency)

		// Settings
		if tr.Settings != nil {
			p.TransformSettingsDatesAsEpochMillis = sqlNullBool(tr.Settings.DatesAsEpochMillis)
			p.TransformSettingsDocsPerSecond = sqlNullFloat64(tr.Settings.DocsPerSecond)
			p.TransformSettingsAlignCheckpoints = sqlNullBool(tr.Settings.AlignCheckpoints)
			p.TransformSettingsMaxPageSearchSize = sqlNullInt64(tr.Settings.MaxPageSearchSize)
			p.TransformSettingsUsePointInTime = sqlNullBool(tr.Settings.UsePointInTime)
			p.TransformSettingsDeduceMappings = sqlNullBool(tr.Settings.DeduceMappings)
			p.TransformSettingsUnattended = sqlNullBool(tr.Settings.Unattended)
		}

		// Meta
		p.TransformMeta = jsonNullString(tr.Meta)

		// Retention policy
		if tr.RetentionPolicy != nil && tr.RetentionPolicy.Time != nil {
			p.TransformRetentionPolicyTimeField = sqlNullString(tr.RetentionPolicy.Time.Field)
			p.TransformRetentionPolicyTimeMaxAge = sqlNullString(tr.RetentionPolicy.Time.MaxAge)
		}

		// Sync
		if tr.Sync != nil && tr.Sync.Time != nil {
			if tr.Sync.Time.Field != nil {
				p.TransformSyncTimeField = sqlNullString(tr.Sync.Time.Field)
			}

			if tr.Sync.Time.Delay != nil {
				p.TransformSyncTimeDelay = sqlNullString(tr.Sync.Time.Delay)
			}
		}
	}
	if t.Manifest != nil {
		p.ManifestStart = sqlNullBool(t.Manifest.Start)
		if t.Manifest.DestinationIndexTemplate != nil {
			p.ManifestDestinationIndexTemplateMappings = jsonNullString(t.Manifest.DestinationIndexTemplate.Mappings)
			p.ManifestDestinationIndexTemplateSettings = jsonNullString(t.Manifest.DestinationIndexTemplate.Settings)
		}
	}
	id, err := q.InsertTransform(ctx, p)
	if err != nil {
		return 0, err
	}

	// Transform destination aliases.
	if t.Transform != nil && t.Transform.Dest != nil {
		for _, alias := range t.Transform.Dest.Aliases {
			_, err = q.InsertTransformDestAlias(ctx, database.InsertTransformDestAliasParams{
				TransformID:    id,
				Alias:          sqlNullString(alias.Alias),
				MoveOnCreation: sqlNullBool(alias.MoveOnCreation),
			})
			if err != nil {
				return 0, err
			}
		}
	}

	return id, nil
}

func sqlNullString(s *string) sql.NullString {
	if s == nil {
		return sql.NullString{}
	}
	return sql.NullString{
		String: *s,
		Valid:  true,
	}
}

func sqlStringEmtpyIsNull(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{
		String: s,
		Valid:  true,
	}
}

func sqlNullInt64(i *int) sql.NullInt64 {
	if i == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{
		Int64: int64(*i),
		Valid: true,
	}
}

func sqlNullFloat64(f *float64) sql.NullFloat64 {
	if f == nil {
		return sql.NullFloat64{}
	}
	return sql.NullFloat64{
		Float64: *f,
		Valid:   true,
	}
}

func sqlNullBool(b *bool) sql.NullBool {
	if b == nil {
		return sql.NullBool{}
	}
	return sql.NullBool{
		Bool:  *b,
		Valid: true,
	}
}

func jsonNullString(v any) sql.NullString {
	val := reflect.ValueOf(v)
	if !val.IsValid() || val.IsZero() {
		return sql.NullString{}
	}
	if val.Kind() == reflect.Slice && val.Len() == 0 {
		return sql.NullString{}
	}

	j, _ := json.Marshal(v)
	return sql.NullString{
		String: string(j),
		Valid:  true,
	}
}

// txDone finalizes the transaction by committing if no error occurred.
// If an error exists, it rolls back and joins errors from rollback and original.
func txDone(tx *sql.Tx, err *error) {
	if *err == nil {
		*err = tx.Commit()
		return
	}

	*err = errors.Join(*err, tx.Rollback())
}
