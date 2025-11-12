-- name: InsertIntegration :one
INSERT INTO integrations (name, dir_name, title, version, description, type,
                          format_version, license, release,
                          policy_templates_behavior,
                          conditions_elastic_subscription,
                          conditions_kibana_version, source_license,
                          owner_github, owner_type,
                          elasticsearch_privileges_cluster,
                          agent_privileges_root,
                          file_path)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
        ?, ?) RETURNING id;

-- name: InsertPolicyTemplate :one
INSERT INTO policy_templates (integration_id, name, title, description, type,
                              deployment_modes_default_enabled,
                              deployment_modes_agentless_enabled,
                              deployment_modes_agentless_is_default,
                              deployment_modes_agentless_organization,
                              deployment_modes_agentless_division,
                              deployment_modes_agentless_team,
                              deployment_modes_agentless_resources_requests_memory,
                              deployment_modes_agentless_resources_requests_cpu)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id;


-- name: InsertPolicyTemplateInput :one
INSERT INTO policy_template_inputs (policy_template_id, type, title,
                                    description,
                                    input_group, template_path, multi)
VALUES (?, ?, ?, ?, ?, ?, ?) RETURNING id;

-- name: InsertDataStream :one
INSERT INTO data_streams (integration_id, name, dataset, dataset_is_prefix,
                          ilm_policy, release, title, type,
                          elasticsearch_index_mode, elasticsearch_source_mode,
                          elasticsearch_dynamic_dataset,
                          elasticsearch_dynamic_namespace,
                          elasticsearch_privileges_properties,
                          elasticsearch_index_template_settings,
                          elasticsearch_index_template_mappings,
                          elasticsearch_index_template_ingest_pipeline_name,
                          elasticsearch_index_template_data_stream_hidden,
                          file_path)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id;

-- name: InsertStream :one
INSERT INTO streams (data_stream_id, input, description, title, template_path,
                     enabled)
VALUES (?, ?, ?, ?, ?, ?) RETURNING id;

-- name: InsertVar :one
INSERT INTO vars (name,
                  default_value,
                  description,
                  type,
                  title,
                  multi,
                  required,
                  secret,
                  show_user,
                  hide_in_deployment_modes,
                  file_path,
                  line_number,
                  col)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id;

-- name: InsertIntegrationVar :exec
INSERT INTO integration_vars (integration_id, var_id)
VALUES (?, ?);

-- name: InsertPolicyTemplateVar :exec
INSERT INTO policy_template_vars (policy_template_id, var_id)
VALUES (?, ?);

-- name: InsertPolicyTemplateInputVars :exec
INSERT INTO policy_template_input_vars (policy_template_input_id, var_id)
VALUES (?, ?);

-- name: InsertStreamVar :exec
INSERT INTO stream_vars (stream_id, var_id)
VALUES (?, ?);

-- name: InsertField :one
INSERT INTO fields (name, type, description, value, example, pattern,
                    date_format,
                    analyzer, search_analyzer,
                    ignore_above, multi_fields, enabled, dynamic, indexed,
                    doc_values, copy_to, scaling_factor, alias_target_path,
                    normalize, normalizer, null_value,
                    dimension, metric_type, external,
                    yaml_path, file_path, line_number, col)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
        ?, ?, ?, ?) RETURNING id;

-- name: InsertDataStreamField :exec
INSERT INTO data_stream_fields (data_stream_id, field_id, fields_file_name)
VALUES (?, ?, ?);

-- name: InsertTransformField :exec
INSERT INTO transform_fields (transform_id, field_id)
VALUES (?, ?);

-- name: InsertTransform :one
INSERT INTO transforms (integration_id, name, transform_source_index,
                        transform_source_query,
                        transform_source_runtime_mappings, transform_dest_index,
                        transform_dest_pipeline,
                        transform_pivot_group_by, transform_pivot_aggregations,
                        transform_pivot_aggs,
                        transform_latest_sort, transform_latest_unique_key,
                        transform_description,
                        transform_frequency,
                        transform_settings_dates_as_epoch_millis,
                        transform_settings_docs_per_second,
                        transform_settings_align_checkpoints,
                        transform_settings_max_page_search_size,
                        transform_settings_use_point_in_time,
                        transform_settings_deduce_mappings,
                        transform_settings_unattended,
                        transform_meta, transform_retention_policy_time_field,
                        transform_retention_policy_time_max_age,
                        transform_sync_time_field,
                        transform_sync_time_delay,
                        manifest_destination_index_template_mappings,
                        manifest_destination_index_template_settings,
                        manifest_start, file_path)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
        ?, ?, ?, ?, ?, ?) RETURNING id;

-- name: InsertIntegrationCategory :exec
INSERT INTO integration_categories (integration_id, category)
VALUES (?, ?);

-- name: InsertPolicyTemplateCategory :exec
INSERT INTO policy_template_categories (policy_template_id, category)
VALUES (?, ?);

-- name: InsertPolicyTemplateDataStream :exec
INSERT INTO policy_template_data_streams (policy_template_id, data_stream_name)
VALUES (?, ?);

-- name: InsertIntegrationIcon :one
INSERT INTO integration_icons (integration_id, src, title, size, type, dark_mode)
VALUES (?, ?, ?, ?, ?, ?) RETURNING id;

-- name: InsertIntegrationScreenshot :one
INSERT INTO integration_screenshots (integration_id, src, title, size, type)
VALUES (?, ?, ?, ?, ?) RETURNING id;

-- name: InsertPolicyTemplateIcon :one
INSERT INTO policy_template_icons (policy_template_id, src, title, size, type, dark_mode)
VALUES (?, ?, ?, ?, ?, ?) RETURNING id;
