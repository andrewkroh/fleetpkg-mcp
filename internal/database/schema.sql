-- Fleet Integration Package Database Schema
-- SQLite schema for storing Fleet integration package data

-- Main table storing integration package information. Each integration represents a complete Fleet package.
CREATE TABLE IF NOT EXISTS integrations (
    id INTEGER PRIMARY KEY AUTOINCREMENT, -- unique identifier
    name TEXT NOT NULL, -- name of the package
    dir_name TEXT UNIQUE NOT NULL, -- directory name of the package
    title TEXT NOT NULL, -- title of the package
    version TEXT NOT NULL, -- version of the package
    description TEXT NOT NULL, -- description of the package
    type TEXT NOT NULL, -- type of package (e.g. integration)
    format_version TEXT NOT NULL, -- version of the package format
    license TEXT, -- license under which the package is being released (deprecated)
    release TEXT, -- stability of the package (deprecated, use prerelease tags in the version)
    policy_templates_behavior TEXT, -- expected behavior when there are more than one policy template defined
    conditions_elastic_subscription TEXT, -- elastic subscription requirement
    conditions_kibana_version TEXT, -- kibana version requirement
    source_license TEXT, -- source license information
    owner_github TEXT NOT NULL, -- github owner information
    owner_type TEXT NOT NULL, -- describes who owns the package and the level of support that is provided
    elasticsearch_privileges_cluster TEXT, -- cluster privilege requirements (JSON array)
    agent_privileges_root BOOLEAN, -- set to true if collection requires root privileges in the agent
    file_path TEXT NOT NULL -- path to the integration directory
);

-- Policy templates offered by integration packages. Related to integrations via foreign key.
CREATE TABLE IF NOT EXISTS policy_templates (
    id INTEGER PRIMARY KEY AUTOINCREMENT, -- unique identifier
    integration_id INTEGER NOT NULL, -- foreign key to integrations table
    name TEXT NOT NULL, -- name of the policy template
    title TEXT NOT NULL, -- title of the policy template
    description TEXT NOT NULL, -- description of the policy template
    type TEXT, -- type of data stream
    input TEXT, -- input type
    template_path TEXT, -- path to template
    multiple BOOLEAN, -- whether multiple instances are allowed
    fips_compatible BOOLEAN, -- indicate if this package is capable of satisfying FIPS requirements
    deployment_modes_default_enabled BOOLEAN, -- defaults to true in Fleet
    deployment_modes_agentless_enabled BOOLEAN, -- agentless deployment enabled
    deployment_modes_agentless_is_default BOOLEAN, -- use agentless mode by default
    deployment_modes_agentless_organization TEXT, -- responsible organization of the integration
    deployment_modes_agentless_division TEXT, -- division responsible for the integration
    deployment_modes_agentless_team TEXT, -- team responsible for the integration
    deployment_modes_agentless_resources_requests_memory TEXT, -- memory allocation for agentless deployment
    deployment_modes_agentless_resources_requests_cpu TEXT, -- CPU allocation for agentless deployment
    FOREIGN KEY (integration_id) REFERENCES integrations(id)
);

-- Data streams within integration packages. Related to integrations via foreign key.
CREATE TABLE IF NOT EXISTS data_streams (
    id INTEGER PRIMARY KEY AUTOINCREMENT, -- unique identifier
    integration_id INTEGER NOT NULL, -- foreign key to integrations table
    name TEXT NOT NULL, -- name of the data stream (key from the map)
    dataset TEXT, -- dataset name
    dataset_is_prefix BOOLEAN, -- whether dataset is a prefix
    ilm_policy TEXT, -- ILM policy name
    release TEXT, -- release information
    title TEXT NOT NULL, -- title of the data stream
    type TEXT, -- type of the data stream
    elasticsearch_index_mode TEXT, -- index mode setting
    elasticsearch_source_mode TEXT, -- source mode setting
    elasticsearch_dynamic_dataset BOOLEAN, -- dynamic dataset setting
    elasticsearch_dynamic_namespace BOOLEAN, -- dynamic namespace setting
    elasticsearch_privileges_properties TEXT, -- properties privileges (JSON array)
    elasticsearch_index_template_settings TEXT, -- index template settings (JSON)
    elasticsearch_index_template_mappings TEXT, -- index template mappings (JSON)
    elasticsearch_index_template_ingest_pipeline_name TEXT, -- ingest pipeline name
    elasticsearch_index_template_data_stream_hidden BOOLEAN, -- data stream hidden setting
    file_path TEXT NOT NULL, -- path to the data stream directory
    FOREIGN KEY (integration_id) REFERENCES integrations(id)
);

-- Individual streams within data stream manifests. Related to data_streams via foreign key.
CREATE TABLE IF NOT EXISTS streams (
    id INTEGER PRIMARY KEY AUTOINCREMENT, -- unique identifier
    data_stream_id INTEGER NOT NULL, -- foreign key to data_streams table
    input TEXT NOT NULL, -- input type
    description TEXT NOT NULL, -- description of the stream
    title TEXT NOT NULL, -- title of the stream
    template_path TEXT, -- path to the template
    enabled BOOLEAN, -- whether the stream is enabled
    FOREIGN KEY (data_stream_id) REFERENCES data_streams(id)
);

-- Configuration variables used throughout the package. This is a master table for all variables.
CREATE TABLE IF NOT EXISTS vars (
    id INTEGER PRIMARY KEY AUTOINCREMENT, -- unique identifier
    name TEXT NOT NULL , -- variable name
    default_value TEXT, -- default value(s) for the variable (JSON)
    description TEXT, -- short description of the variable
    type TEXT NOT NULL, -- data type of variable (e.g., bool, email, integer, password, select, text, textarea, time_zone, url, yaml)
    title TEXT, -- title of the variable displayed in the UI
    multi BOOLEAN, -- specifies if the variable can contain multiple values
    required BOOLEAN, -- specifies if the variable is required
    secret BOOLEAN, -- indicates that the variable contains sensitive information
    show_user BOOLEAN, -- indicates whether this variable should be shown to the user by default
    hide_in_deployment_modes TEXT, -- deployment modes where this variable should be hidden (JSON array)
    file_path TEXT NOT NULL, -- file path where the variable is defined
    line_number INTEGER NOT NULL, -- line number in the file
    col INTEGER NOT NULL -- character position in the file
);

-- Join table linking integrations to their configuration variables.
CREATE TABLE IF NOT EXISTS integration_vars (
    integration_id INTEGER NOT NULL, -- foreign key to integrations table
    var_id INTEGER NOT NULL, -- foreign key to vars table
    PRIMARY KEY (integration_id, var_id),
    FOREIGN KEY (integration_id) REFERENCES integrations(id),
    FOREIGN KEY (var_id) REFERENCES vars(id)
);

-- Join table linking policy templates to their configuration variables.
CREATE TABLE IF NOT EXISTS policy_template_vars (
    policy_template_id  INTEGER NOT NULL, -- foreign key to policy_templates table
    var_id INTEGER NOT NULL, -- foreign key to vars table
    PRIMARY KEY (policy_template_id, var_id),
    FOREIGN KEY (policy_template_id) REFERENCES policy_templates(id),
    FOREIGN KEY (var_id) REFERENCES vars(id)
);

-- Join table linking policy template inputs to their configuration variables.
CREATE TABLE IF NOT EXISTS policy_template_input_vars (
    policy_template_input_id INTEGER NOT NULL, -- foreign key to policy_template_inputs table
    var_id INTEGER NOT NULL, -- foreign key to vars table
    PRIMARY KEY (policy_template_input_id, var_id),
    FOREIGN KEY (policy_template_input_id) REFERENCES policy_template_inputs(id),
    FOREIGN KEY (var_id) REFERENCES vars(id)
);

-- Join table linking streams to their configuration variables.
CREATE TABLE IF NOT EXISTS stream_vars (
    stream_id  INTEGER NOT NULL, -- foreign key to streams table
    var_id  INTEGER NOT NULL, -- foreign key to vars table
    PRIMARY KEY (stream_id, var_id),
    FOREIGN KEY (stream_id) REFERENCES streams(id),
    FOREIGN KEY (var_id) REFERENCES vars(id)
);

-- Elasticsearch field definitions used in data streams and transforms.
CREATE TABLE IF NOT EXISTS fields (
    id INTEGER PRIMARY KEY AUTOINCREMENT, -- unique identifier
    name TEXT NOT NULL, -- name of the field
    type TEXT, -- type of the field as used in Elasticsearch
    description TEXT, -- description of the field
    value TEXT, -- value of the field
    example TEXT, -- example of the field value
    pattern TEXT, -- regex pattern for the field
    date_format TEXT, -- input format for date fields
    analyzer TEXT, -- analyzer to use for the field
    search_analyzer TEXT, -- search analyzer to use for the field
    ignore_above INTEGER, -- ignore above setting for the field
    multi_fields TEXT, -- multi-fields configuration (JSON)
    enabled BOOLEAN, -- whether the field is enabled
    dynamic TEXT, -- dynamic setting for the field
    indexed BOOLEAN, -- whether the field should be indexed
    doc_values BOOLEAN, -- whether doc values should be stored
    copy_to TEXT, -- copy_to setting
    scaling_factor INTEGER, -- scaling factor for scaled_float fields
    alias_target_path TEXT, -- for alias type fields this is the path to the target field
    normalize TEXT, -- expected ECS normalizations for a field (options are 'array') (JSON)
    normalizer TEXT, -- name of a Elasticsearch normalizer to use
    null_value TEXT, -- null value replacement
    dimension BOOLEAN, -- whether the field is a dimension in TSDB
    metric_type TEXT, -- metric type for TSDB fields
    unit TEXT, -- unit of measurement for the field
    external TEXT, -- external definition source (possible values are 'ecs')
    yaml_path TEXT, -- YAML path to the field definition
    file_path TEXT NOT NULL, -- file path where the field is defined
    line_number INTEGER NOT NULL, -- line number in the file
    col INTEGER NOT NULL -- character position in the file
);

-- Elasticsearch transform configurations within integration packages.
CREATE TABLE IF NOT EXISTS transforms (
    id INTEGER PRIMARY KEY AUTOINCREMENT, -- unique identifier
    integration_id INTEGER NOT NULL, -- foreign key to integrations table
    name TEXT NOT NULL, -- name of the transform (key from the map)
    transform_source_index TEXT NOT NULL, -- source index or indices (JSON)
    transform_source_query TEXT, -- query to filter source documents (JSON)
    transform_source_runtime_mappings TEXT, -- runtime field mappings (JSON)
    transform_dest_index TEXT NOT NULL, -- destination index name
    transform_dest_pipeline TEXT, -- ingest pipeline to use for the destination
    transform_dest_aliases_json TEXT, -- aliases to the destination index (JSON)
    transform_pivot_group_by TEXT, -- grouping configuration (JSON)
    transform_pivot_aggregations TEXT, -- aggregations to perform (JSON)
    transform_pivot_aggs TEXT, -- alternative name for aggregations (JSON)
    transform_latest_sort TEXT, -- sort field for determining the latest documents
    transform_latest_unique_key TEXT, -- unique key fields (JSON array)
    transform_description TEXT, -- description of the transform
    transform_frequency TEXT, -- frequency of the transform execution
    transform_settings_dates_as_epoch_millis BOOLEAN, -- whether dates should be stored as epoch milliseconds
    transform_settings_docs_per_second REAL, -- number of documents processed per second limit
    transform_settings_align_checkpoints BOOLEAN, -- whether checkpoints should be aligned
    transform_settings_max_page_search_size INTEGER, -- maximum page size for search requests
    transform_settings_use_point_in_time BOOLEAN, -- whether to use point-in-time for search requests
    transform_settings_deduce_mappings BOOLEAN, -- whether to deduce mappings automatically
    transform_settings_unattended BOOLEAN, -- whether the transform runs in unattended mode
    transform_meta TEXT, -- arbitrary metadata for the transform (JSON)
    transform_retention_policy_time_field TEXT, -- field used for time-based retention
    transform_retention_policy_time_max_age TEXT, -- maximum age for retaining documents
    transform_sync_time_field TEXT, -- field used for time-based synchronization
    transform_sync_time_delay TEXT, -- delay for synchronization
    manifest_destination_index_template_mappings TEXT, -- destination index template mappings (JSON)
    manifest_destination_index_template_settings TEXT, -- destination index template settings (JSON)
    manifest_start BOOLEAN, -- whether to start the transform
    file_path TEXT NOT NULL, -- path to the transform directory
    FOREIGN KEY (integration_id) REFERENCES integrations(id)
);

-- Input configurations for policy templates. Related to policy_templates via foreign key.
CREATE TABLE IF NOT EXISTS policy_template_inputs (
    id INTEGER PRIMARY KEY AUTOINCREMENT, -- unique identifier
    policy_template_id INTEGER NOT NULL, -- foreign key to policy_templates table
    type TEXT NOT NULL, -- input type
    title TEXT NOT NULL, -- title of the input
    description TEXT NOT NULL, -- description of the input
    input_group TEXT, -- input group classification
    template_path TEXT, -- path to the input template
    multi BOOLEAN, -- whether multiple instances are allowed
    FOREIGN KEY (policy_template_id) REFERENCES policy_templates(id)
);

-- Categories associated with policy templates. Join table for many-to-many relationship.
CREATE TABLE IF NOT EXISTS policy_template_categories (
    policy_template_id INTEGER NOT NULL, -- foreign key to policy_templates table
    category TEXT NOT NULL, -- category name
    PRIMARY KEY (policy_template_id, category),
    FOREIGN KEY (policy_template_id) REFERENCES policy_templates(id)
);

-- Data streams associated with policy templates. Join table for many-to-many relationship.
CREATE TABLE IF NOT EXISTS policy_template_data_streams (
    policy_template_id INTEGER NOT NULL, -- foreign key to policy_templates table
    data_stream_name TEXT NOT NULL, -- name of the data stream
    PRIMARY KEY (policy_template_id, data_stream_name),
    FOREIGN KEY (policy_template_id) REFERENCES policy_templates(id)
);

-- Categories associated with integrations. Join table for many-to-many relationship.
CREATE TABLE IF NOT EXISTS integration_categories (
    integration_id INTEGER NOT NULL, -- foreign key to integrations table
    category TEXT NOT NULL, -- category name
    PRIMARY KEY (integration_id, category),
    FOREIGN KEY (integration_id) REFERENCES integrations(id)
);

-- Icons associated with integrations. Related to integrations via foreign key.
CREATE TABLE IF NOT EXISTS integration_icons (
    id INTEGER PRIMARY KEY AUTOINCREMENT, -- unique identifier
    integration_id INTEGER NOT NULL, -- foreign key to integrations table
    src TEXT, -- source path of the icon
    title TEXT, -- title of the icon
    size TEXT, -- size specification
    type TEXT, -- MIME type of the icon
    dark_mode BOOLEAN, -- whether the icon is for dark mode
    FOREIGN KEY (integration_id) REFERENCES integrations(id)
);

-- Screenshots associated with integrations. Related to integrations via foreign key.
CREATE TABLE IF NOT EXISTS integration_screenshots (
    id INTEGER PRIMARY KEY AUTOINCREMENT, -- unique identifier
    integration_id INTEGER NOT NULL, -- foreign key to integrations table
    src TEXT, -- source path of the screenshot
    title TEXT, -- title of the screenshot
    size TEXT, -- size specification
    type TEXT, -- MIME type of the screenshot
    FOREIGN KEY (integration_id) REFERENCES integrations(id)
);

-- Icons associated with policy templates. Related to policy_templates via foreign key.
CREATE TABLE IF NOT EXISTS policy_template_icons (
    id INTEGER PRIMARY KEY AUTOINCREMENT, -- unique identifier
    policy_template_id INTEGER NOT NULL, -- foreign key to policy_templates table
    src TEXT, -- source path of the icon
    title TEXT, -- title of the icon
    size TEXT, -- size specification
    type TEXT, -- MIME type of the icon
    dark_mode BOOLEAN, -- whether the icon is for dark mode
    FOREIGN KEY (policy_template_id) REFERENCES policy_templates(id)
);

-- Screenshots associated with policy templates. Related to policy_templates via foreign key.
CREATE TABLE IF NOT EXISTS policy_template_screenshots (
    id INTEGER PRIMARY KEY AUTOINCREMENT, -- unique identifier
    policy_template_id INTEGER NOT NULL, -- foreign key to policy_templates table
    src TEXT, -- source path of the screenshot
    title TEXT, -- title of the screenshot
    size TEXT, -- size specification
    type TEXT, -- MIME type of the screenshot
    FOREIGN KEY (policy_template_id) REFERENCES policy_templates(id)
);

-- Options for select-type variables. Related to vars via foreign key.
CREATE TABLE IF NOT EXISTS var_options (
    id INTEGER PRIMARY KEY AUTOINCREMENT, -- unique identifier
    var_id INTEGER NOT NULL, -- foreign key to vars table
    value TEXT, -- option value
    text TEXT, -- display text for the option
    FOREIGN KEY (var_id) REFERENCES vars(id)
);

-- Join table linking data streams to their field definitions.
CREATE TABLE IF NOT EXISTS data_stream_fields (
    data_stream_id INTEGER NOT NULL, -- foreign key to data_streams table
    field_id INTEGER NOT NULL, -- foreign key to fields table
    fields_file_name TEXT NOT NULL, -- name of the fields file
    PRIMARY KEY (data_stream_id, field_id),
    FOREIGN KEY (data_stream_id) REFERENCES data_streams(id),
    FOREIGN KEY (field_id) REFERENCES fields(id)
);

-- Join table linking transforms to their field definitions.
CREATE TABLE IF NOT EXISTS transform_fields (
    transform_id INTEGER NOT NULL, -- foreign key to transforms table
    field_id INTEGER NOT NULL, -- foreign key to fields table
    PRIMARY KEY (transform_id, field_id),
    FOREIGN KEY (transform_id) REFERENCES transforms(id),
    FOREIGN KEY (field_id) REFERENCES fields(id)
);

-- Aliases for transform destination indices. Related to transforms via foreign key.
CREATE TABLE IF NOT EXISTS transform_dest_aliases (
    id INTEGER PRIMARY KEY AUTOINCREMENT, -- unique identifier
    transform_id INTEGER NOT NULL, -- foreign key to transforms table
    alias TEXT, -- name of the alias
    move_on_creation BOOLEAN, -- whether the destination index should be the only index in this alias
    FOREIGN KEY (transform_id) REFERENCES transforms(id)
);

-- Fields associated with package discovery capabilities. Related to integrations via foreign key.
CREATE TABLE IF NOT EXISTS discovery_fields (
    id INTEGER PRIMARY KEY AUTOINCREMENT, -- unique identifier
    integration_id INTEGER NOT NULL, -- foreign key to integrations table
    name TEXT, -- name of the field
    FOREIGN KEY (integration_id) REFERENCES integrations(id)
);

-- Build configuration for integration packages. Related to integrations via foreign key.
CREATE TABLE IF NOT EXISTS build_manifests (
    id INTEGER PRIMARY KEY AUTOINCREMENT, -- unique identifier
    integration_id INTEGER NOT NULL, -- foreign key to integrations table
    dependencies_ecs_reference TEXT, -- ECS source reference
    dependencies_ecs_import_mappings BOOLEAN, -- whether to import common used dynamic templates and properties
    file_path TEXT NOT NULL, -- path to the build.yml file
    FOREIGN KEY (integration_id) REFERENCES integrations(id)
);

-- Version history for integration packages. Related to integrations via foreign key.
CREATE TABLE IF NOT EXISTS changelogs (
    id INTEGER PRIMARY KEY AUTOINCREMENT, -- unique identifier
    integration_id INTEGER NOT NULL, -- foreign key to integrations table
    file_path TEXT NOT NULL, -- path to the changelog file
    FOREIGN KEY (integration_id) REFERENCES integrations(id)
);

-- Individual releases within changelogs. Related to changelogs via foreign key.
CREATE TABLE IF NOT EXISTS releases (
    id INTEGER PRIMARY KEY AUTOINCREMENT, -- unique identifier
    changelog_id INTEGER NOT NULL, -- foreign key to changelogs table
    version TEXT, -- version of the release
    file_path TEXT NOT NULL, -- file path where the release is defined
    line_number INTEGER, -- line number in the file
    col INTEGER, -- character position in the file
    FOREIGN KEY (changelog_id) REFERENCES changelogs(id)
);

-- Individual changes within releases. Related to releases via foreign key.
CREATE TABLE IF NOT EXISTS changes (
    id INTEGER PRIMARY KEY AUTOINCREMENT, -- unique identifier
    release_id INTEGER NOT NULL, -- foreign key to releases table
    description TEXT, -- description of the change
    type TEXT, -- type of change (e.g., enhancement, bugfix)
    link TEXT, -- link to more information about the change
    file_path TEXT NOT NULL, -- file path where the change is defined
    line_number INTEGER, -- line number in the file
    col INTEGER, -- character position in the file
    FOREIGN KEY (release_id) REFERENCES releases(id)
);

-- Ingest pipeline configurations within data streams. Related to data_streams via foreign key.
CREATE TABLE IF NOT EXISTS ingest_pipelines (
    id INTEGER PRIMARY KEY AUTOINCREMENT, -- unique identifier
    data_stream_id INTEGER NOT NULL, -- foreign key to data_streams table
    name TEXT, -- name of the pipeline (key from the map)
    description TEXT, -- description of the ingest pipeline
    version INTEGER, -- version number used by external systems to track ingest pipelines
    meta TEXT, -- optional metadata about the ingest pipeline (JSON)
    file_path TEXT NOT NULL, -- path to the ingest node pipeline file
    FOREIGN KEY (data_stream_id) REFERENCES data_streams(id)
);

-- Ingest processors within pipelines. Related to ingest_pipelines via foreign key.
CREATE TABLE IF NOT EXISTS ingest_processors (
    id INTEGER PRIMARY KEY AUTOINCREMENT, -- unique identifier
    ingest_pipeline_id INTEGER NOT NULL, -- foreign key to ingest_pipelines table
    type TEXT NOT NULL, -- ingest processor type
    attributes JSON, -- processor configuration (JSON)
    json_pointer TEXT NOT NULL, -- JSON Pointer (RFC 6901) location within the pipeline (e.g. '/processors/12/append' or '/on_failure/1/append').
    file_path TEXT NOT NULL, -- file path where the processor is defined
    line_number INTEGER NOT NULL, -- line number in the file
    col INTEGER NOT NULL, -- character position in the file
    FOREIGN KEY (ingest_pipeline_id) REFERENCES ingest_pipelines(id)
);

-- Sample event data for data streams. Related to data_streams via foreign key.
CREATE TABLE IF NOT EXISTS sample_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT, -- unique identifier
    data_stream_id INTEGER NOT NULL, -- foreign key to data_streams table
    event TEXT, -- sample event data (JSON)
    file_path TEXT NOT NULL, -- path to the sample event file
    FOREIGN KEY (data_stream_id) REFERENCES data_streams(id)
);
