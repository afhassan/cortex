# Changelog

## master / unreleased
* [CHANGE] StoreGateway/Alertmanager: Add default 5s connection timeout on client. #6603
* [CHANGE] Ingester: Remove EnableNativeHistograms config flag and instead gate keep through new per-tenant limit at ingestion. #6718
* [CHANGE] Validate a tenantID when to use a single tenant resolver. #6727
* [FEATURE] Query Frontend: Add dynamic interval size for query splitting. This is enabled by configuring experimental flags `querier.max-shards-per-query` and/or `querier.max-fetched-data-duration-per-query`. The split interval size is dynamically increased to maintain a number of shards and total duration fetched below the configured values. #6458
* [FEATURE] Querier/Ruler: Add `query_partial_data` and `rules_partial_data` limits to allow queries/rules to be evaluated with data from a single zone, if other zones are not available. #6526
* [FEATURE] Update prometheus alertmanager version to v0.28.0 and add new integration msteamsv2, jira, and rocketchat. #6590
* [FEATURE] Ingester/StoreGateway: Add `ResourceMonitor` module in Cortex, and add `ResourceBasedLimiter` in Ingesters and StoreGateways. #6674
* [FEATURE] Ingester: Support out-of-order native histogram ingestion. It automatically enabled when `-ingester.out-of-order-time-window > 0` and `-blocks-storage.tsdb.enable-native-histograms=true`. #6626 #6663
* [FEATURE] Ruler: Add support for percentage based sharding for rulers. #6680
* [FEATURE] Ruler: Add support for group labels. #6665
* [FEATURE] Query federation: Introduce a regex tenant resolver to allow regex in `X-Scope-OrgID` value. #6713
- Add an experimental `tenant-federation.regex-matcher-enabled` flag. If it enabled, user can input regex to `X-Scope-OrgId`, the matched tenantIDs are automatically involved. The user discovery is based on scanning block storage, so new users can get queries after uploading a block (generally 2h).
- Add an experimental `tenant-federation.user-sync-interval` flag, it specifies how frequently to scan users. The scanned users are used to calculate matched tenantIDs.
* [FEATURE] Experimental Support Parquet format: Implement parquet converter service to convert a TSDB block into Parquet and Parquet Queryable. #6716 #6743
* [FEATURE] Distributor/Ingester: Implemented experimental feature to use gRPC stream connection for push requests. This can be enabled by setting `-distributor.use-stream-push=true`. #6580
* [FEATURE] Compactor: Add support for percentage based sharding for compactors. #6738
* [FEATURE] Querier: Allow choosing PromQL engine via header. #6777
* [FEATURE] Querier: Support for configuring query optimizers and enabling XFunctions in the Thanos engine. #6873
* [FEATURE] Query Frontend: Add support /api/v1/format_query API for formatting queries. #6893
* [ENHANCEMENT] Ruler: Emit an error message when the rule synchronization fails. #6902
* [ENHANCEMENT] Querier: Support snappy and zstd response compression for `-querier.response-compression` flag. #6848
* [ENHANCEMENT] Tenant Federation: Add a # of query result limit logic when the `-tenant-federation.regex-matcher-enabled` is enabled. #6845
* [ENHANCEMENT] Query Frontend: Add a `cortex_slow_queries_total` metric to track # of slow queries per user. #6859
* [ENHANCEMENT] Query Frontend: Change to return 400 when the tenant resolving fail. #6715
* [ENHANCEMENT] Querier: Support query parameters to metadata api (/api/v1/metadata) to allow user to limit metadata to return. Add a `-ingester.return-all-metadata` flag to make the metadata API run when the deployment. Please set this flag to `false` to use the metadata API with the limits later. #6681 #6744
* [ENHANCEMENT] Ingester: Add a `cortex_ingester_active_native_histogram_series` metric to track # of active NH series. #6695
* [ENHANCEMENT] Query Frontend: Add new limit `-frontend.max-query-response-size` for total query response size after decompression in query frontend. #6607
* [ENHANCEMENT] Alertmanager: Add nflog and silences maintenance metrics. #6659
* [ENHANCEMENT] Querier: limit label APIs to query only ingesters if `start` param is not been specified. #6618
* [ENHANCEMENT] Alertmanager: Add new limits `-alertmanager.max-silences-count` and `-alertmanager.max-silences-size-bytes` for limiting silences per tenant. #6605
* [ENHANCEMENT] Update prometheus version to v3.1.0. #6583
* [ENHANCEMENT] Add `compactor.auto-forget-delay` for compactor to auto forget compactors after X minutes without heartbeat. #6533
* [ENHANCEMENT] StoreGateway: Emit more histogram buckets on the `cortex_querier_storegateway_refetches_per_query` metric. #6570
* [ENHANCEMENT] Querier: Apply bytes limiter to LabelNames and LabelValuesForLabelNames. #6568
* [ENHANCEMENT] Query Frontend: Add a `too_many_tenants` reason label value to `cortex_rejected_queries_total` metric to track the rejected query count due to the # of tenant limits. #6569
* [ENHANCEMENT] Alertmanager: Add receiver validations for msteamsv2 and rocketchat. #6606
* [ENHANCEMENT] Query Frontend: Add a `-frontend.enabled-ruler-query-stats` flag to configure whether to report the query stats log for queries coming from the Ruler. #6504
* [ENHANCEMENT] OTLP: Support otlp metadata ingestion. #6617
* [ENHANCEMENT] AlertManager: Add `keep_instance_in_the_ring_on_shutdown` and `tokens_file_path` configs for alertmanager ring. #6628
* [ENHANCEMENT] Querier: Add metric and enhanced logging for query partial data. #6676
* [ENHANCEMENT] Ingester: Push request should fail when label set is out of order #6746
* [ENHANCEMENT] Querier: Add `querier.ingester-query-max-attempts` to retry on partial data. #6714
* [ENHANCEMENT] Distributor: Add min/max schema validation for Native Histogram. #6766
* [ENHANCEMENT] Ingester: Handle runtime errors in query path #6769
* [ENHANCEMENT] Compactor: Support metadata caching bucket for Cleaner. Can be enabled via `-compactor.cleaner-caching-bucket-enabled` flag. #6778
* [ENHANCEMENT] Distributor: Add ingestion rate limit for Native Histogram. #6794
* [ENHANCEMENT] Ingester: Add active series limit specifically for Native Histogram. #6796
* [ENHANCEMENT] Compactor, Store Gateway: Introduce user scanner strategy and user index. #6780
* [ENHANCEMENT] Querier: Support chunks cache for parquet queryable. #6805
* [ENHANCEMENT] Parquet Storage: Add some metrics for parquet blocks and converter. #6809 #6821
* [ENHANCEMENT] Compactor: Optimize cleaner run time. #6815
* [ENHANCEMENT] Parquet Storage: Allow percentage based dynamic shard size for Parquet Converter. #6817
* [ENHANCEMENT] Query Frontend: Enhance the performance of the JSON codec. #6816
* [ENHANCEMENT] Compactor: Emit partition metrics separate from cleaner job. #6827
* [ENHANCEMENT] Metadata Cache: Support inmemory and multi level cache backend. #6829
* [ENHANCEMENT] Store Gateway: Allow to ignore syncing blocks older than certain time using `ignore_blocks_before`. #6830
* [ENHANCEMENT] Distributor: Add native histograms max sample size bytes limit validation. #6834
* [ENHANCEMENT] Querier: Support caching parquet labels file in parquet queryable. #6835
* [ENHANCEMENT] Querier: Support query limits in parquet queryable. #6870
* [ENHANCEMENT] Ring: Add zone label to ring_members metric. #6900
* [ENHANCEMENT] Ingester: Add new metric `cortex_ingester_push_errors_total` to track reasons for ingester request failures. #6901
* [ENHANCEMENT] Ring: Expose `detailed_metrics_enabled` for all rings. Default true. #6926
* [ENHANCEMENT] Parquet Storage: Allow Parquet Queryable to disable fallback to Store Gateway. #6920
* [ENHANCEMENT] Query Frontend: Add a `format_query` label value to the `op` label at `cortex_query_frontend_queries_total` metric. #6925
* [ENHANCEMENT] API: add request ID injection to context to enable tracking requests across downstream services. #6895
* [BUGFIX] Ingester: Avoid error or early throttling when READONLY ingesters are present in the ring #6517
* [BUGFIX] Ingester: Fix labelset data race condition. #6573
* [BUGFIX] Compactor: Cleaner should not put deletion marker for blocks with no-compact marker. #6576
* [BUGFIX] Compactor: Cleaner would delete bucket index when there is no block in bucket store. #6577
* [BUGFIX] Querier: Fix marshal native histogram with empty bucket when protobuf codec is enabled. #6595
* [BUGFIX] Query Frontend: Fix samples scanned and peak samples query stats when query hits results cache. #6591
* [BUGFIX] Query Frontend: Fix panic caused by nil pointer dereference. #6609
* [BUGFIX] Ingester: Add check to avoid query 5xx when closing tsdb. #6616
* [BUGFIX] Querier: Fix panic when marshaling QueryResultRequest. #6601
* [BUGFIX] Ingester: Avoid resharding for query when restart readonly ingesters. #6642
* [BUGFIX] Query Frontend: Fix query frontend per `user` metrics clean up. #6698
* [BUGFIX] Add `__markers__` tenant ID validation. #6761
* [BUGFIX] Ring: Fix nil pointer exception when token is shared. #6768
* [BUGFIX] Fix race condition in active user. #6773
* [BUGFIX] Ruler: Prevent counting 2xx and 4XX responses as failed writes. #6785
* [BUGFIX] Ingester: Allow shipper to skip corrupted blocks. #6786
* [BUGFIX] Compactor: Delete the prefix `blocks_meta` from the metadata fetcher metrics. #6832
* [BUGFIX] Store Gateway: Avoid race condition by deduplicating entries in bucket stores user scan. #6863
* [BUGFIX] Runtime-config: Change to check tenant limit validation when loading runtime config only for `all`, `distributor`, `querier`, and `ruler` targets. #6880

## 1.19.0 2025-02-27

* [CHANGE] Deprecate `-blocks-storage.tsdb.wal-compression-enabled` flag (use `blocks-storage.tsdb.wal-compression-type` instead). #6529
* [CHANGE] OTLP: Change OTLP handler to be consistent with the Prometheus OTLP handler. #6272
- `target_info` metric is enabled by default and can be disabled via `-distributor.otlp.disable-target-info=true` flag
- Convert all attributes to labels is disabled by default and can be enabled via `-distributor.otlp.convert-all-attributes=true` flag
- You can specify the attributes converted to labels via `-distributor.promote-resource-attributes` flag. Supported only if `-distributor.otlp.convert-all-attributes=false`
* [CHANGE] Change all max async concurrency default values `50` to `3` #6268
* [CHANGE] Change default value of `-blocks-storage.bucket-store.index-cache.multilevel.max-async-concurrency` from `50` to `3` #6265
* [CHANGE] Enable Compactor and Alertmanager in target all. #6204
* [CHANGE] Update the `cortex_ingester_inflight_push_requests` metric to represent the maximum number of inflight requests recorded in the last minute. #6437
* [CHANGE] gRPC Client: Expose connection timeout and set default to value to 5s. #6523
* [FEATURE] Ruler: Add an experimental flag `-ruler.query-response-format` to retrieve query response as a proto format. #6345
* [FEATURE] Ruler: Pagination support for List Rules API. #6299
* [FEATURE] Query Frontend/Querier: Add protobuf codec `-api.querier-default-codec` and the option to choose response compression type `-querier.response-compression`. #5527
* [FEATURE] Ruler: Experimental: Add `ruler.frontend-address` to allow query to query frontends instead of ingesters. #6151
* [FEATURE] Ruler: Minimize chances of missed rule group evaluations that can occur due to OOM kills, bad underlying nodes, or due to an unhealthy ruler that appears in the ring as healthy. This feature is enabled via `-ruler.enable-ha-evaluation` flag. #6129
* [FEATURE] Store Gateway: Add an in-memory chunk cache. #6245
* [FEATURE] Chunk Cache: Support multi level cache and add metrics. #6249
* [FEATURE] Distributor: Accept multiple HA Tracker pairs in the same request. #6256
* [FEATURE] Ruler: Add support for per-user external labels #6340
* [FEATURE] Query Frontend: Support a metadata federated query when `-tenant-federation.enabled=true`. #6461
* [FEATURE] Query Frontend: Support an exemplar federated query when `-tenant-federation.enabled=true`. #6455
* [FEATURE] Ingester/StoreGateway: Add support for cache regex query matchers via `-ingester.matchers-cache-max-items` and `-blocks-storage.bucket-store.matchers-cache-max-items`. #6477 #6491
* [ENHANCEMENT] Query Frontend: Add more operation label values to the `cortex_query_frontend_queries_total` metric. #6519
* [ENHANCEMENT] Query Frontend: Add a `source` label to query stat metrics. #6470
* [ENHANCEMENT] Query Frontend: Add a flag `-tenant-federation.max-tenant` to limit the number of tenants for federated query. #6493
* [ENHANCEMENT] Querier: Add a `-tenant-federation.max-concurrent` flags to configure the number of worker processing federated query and add a `cortex_querier_federated_tenants_per_query` histogram to track the number of tenants per query. #6449
* [ENHANCEMENT] Query Frontend: Add a number of series in the query response to the query stat log. #6423
* [ENHANCEMENT] Store Gateway: Add a hedged request to reduce the tail latency. #6388
* [ENHANCEMENT] Ingester: Add metrics to track succeed/failed native histograms. #6370
* [ENHANCEMENT] Query Frontend/Querier: Add an experimental flag `-querier.enable-promql-experimental-functions` to enable experimental promQL functions. #6355
* [ENHANCEMENT] OTLP: Add `-distributor.otlp-max-recv-msg-size` flag to limit OTLP request size in bytes. #6333
* [ENHANCEMENT] S3 Bucket Client: Add a list objects version configs to configure list api object version. #6280
* [ENHANCEMENT] OpenStack Swift: Add application credential configs for Openstack swift object storage backend. #6255
* [ENHANCEMENT] Query Frontend: Add new query stats metrics `cortex_query_samples_scanned_total` and `cortex_query_peak_samples` to track scannedSamples and peakSample per user. #6228
* [ENHANCEMENT] Ingester: Add option `ingester.disable-chunk-trimming` to disable chunk trimming. #6300
* [ENHANCEMENT] Ingester: Add `blocks-storage.tsdb.wal-compression-type` to support zstd wal compression type. #6232
* [ENHANCEMENT] Query Frontend: Add info field to query response. #6207
* [ENHANCEMENT] Query Frontend: Add peakSample in query stats response. #6188
* [ENHANCEMENT] Ruler: Add new ruler metric `cortex_ruler_rule_groups_in_store` that is the total rule groups per tenant in store, which can be used to compare with `cortex_prometheus_rule_group_rules` to count the number of rule groups that are not loaded by a ruler. #5869
* [ENHANCEMENT] Ingester/Ring: New `READONLY` status on ring to be used by Ingester. New ingester API to change mode of ingester #6163
* [ENHANCEMENT] Ruler: Add query statistics metrics when --ruler.query-stats-enabled=true. #6173
* [ENHANCEMENT] Ingester: Add new API `/ingester/all_user_stats` which shows loaded blocks, active timeseries and ingestion rate for a specific ingester. #6178
* [ENHANCEMENT] Distributor: Add new `cortex_reduced_resolution_histogram_samples_total` metric to track the number of histogram samples which resolution was reduced. #6182
* [ENHANCEMENT] StoreGateway: Implement metadata API limit in queryable. #6195
* [ENHANCEMENT] Ingester: Add matchers to ingester LabelNames() and LabelNamesStream() RPC. #6209
* [ENHANCEMENT] KV: Add TLS configs to consul. #6374
* [ENHANCEMENT] Ingester/Store Gateway Clients: Introduce an experimental HealthCheck handler to quickly fail requests directed to unhealthy targets. #6225 #6257
* [ENHANCEMENT] Upgrade build image and Go version to 1.23.2. #6261 #6262
* [ENHANCEMENT] Ingester: Introduce a new experimental feature for caching expanded postings on the ingester. #6296
* [ENHANCEMENT] Querier/Ruler: Expose `store_gateway_consistency_check_max_attempts` for max retries when querying store gateway in consistency check. #6276
* [ENHANCEMENT] StoreGateway: Add new `cortex_bucket_store_chunk_pool_inuse_bytes` metric to track the usage in chunk pool. #6310
* [ENHANCEMENT] Distributor: Add new `cortex_distributor_inflight_client_requests` metric to track number of ingester client inflight requests. #6358
* [ENHANCEMENT] Distributor: Expose `cortex_label_size_bytes` native histogram metric. #6372
* [ENHANCEMENT] Add new option `-server.grpc_server-num-stream-workers` to configure the number of worker goroutines that should be used to process incoming streams. #6386
* [ENHANCEMENT] Distributor: Return HTTP 5XX instead of HTTP 4XX when instance limits are hit. #6358
* [ENHANCEMENT] Ingester: Make sure unregistered ingester joining the ring after WAL replay. #6277
* [ENHANCEMENT] Distributor: Add a new `-distributor.num-push-workers` flag to use a goroutine worker pool when sending data from distributor to ingesters. #6406
* [ENHANCEMENT] Ingester: If a limit per label set entry doesn't have any label, use it as the default partition to catch all series that doesn't match any other label sets entries. #6435
* [ENHANCEMENT] Querier: Add new `cortex_querier_codec_response_size` metric to track the size of the encoded query responses from queriers. #6444
* [ENHANCEMENT] Distributor: Added `cortex_distributor_received_samples_per_labelset_total` metric to calculate ingestion rate per label set. #6443
* [ENHANCEMENT] Added metric name in limiter per-metric exceeded errors. #6416
* [ENHANCEMENT] StoreGateway: Added `cortex_bucket_store_indexheader_load_duration_seconds` and `cortex_bucket_store_indexheader_download_duration_seconds` metrics for time of downloading and loading index header files. #6445
* [ENHANCEMENT] Blocks Storage: Allow use of non-dualstack endpoints for S3 blocks storage via `-blocks-storage.s3.disable-dualstack`. #6522
* [BUGFIX] Runtime-config: Handle absolute file paths when working directory is not / #6224
* [BUGFIX] Ruler: Allow rule evaluation to complete during shutdown. #6326
* [BUGFIX] Ring: update ring with new ip address when instance is lost, rejoins, but heartbeat is disabled.  #6271
* [BUGFIX] Ingester: Fix regression on usage of cortex_ingester_queried_chunks. #6398
* [BUGFIX] Ingester: Fix possible race condition when `active series per LabelSet` is configured. #6409
* [BUGFIX] Query Frontend: Fix @ modifier not being applied correctly on sub queries. #6450
* [BUGFIX] Cortex Redis flags with multiple dots #6476
* [BUGFIX] ThanosEngine: Only enable default optimizers. #6776

## 1.18.1 2024-10-14

* [BUGFIX] Backporting upgrade to go 1.22.7 to patch CVE-2024-34155, CVE-2024-34156, CVE-2024-34158 #6217 #6264


## 1.18.0 2024-09-03

* [CHANGE] Ingester: Remove `-querier.query-store-for-labels-enabled` flag. Querying long-term store for labels is always enabled. #5984
* [CHANGE] Server: Instrument `cortex_request_duration_seconds` metric with native histogram. If `native-histograms` feature is enabled in monitoring Prometheus then the metric name needs to be updated in your dashboards. #6056
* [CHANGE] Distributor/Ingester: Change `cortex_distributor_ingester_appends_total`, `cortex_distributor_ingester_append_failures_total`, `cortex_distributor_ingester_queries_total`, and `cortex_distributor_ingester_query_failures_total` metrics to use the ingester ID instead of its IP as the label value. #6078
* [CHANGE] OTLP: Set `AddMetricSuffixes` to true to always enable metric name normalization. #6136
* [CHANGE] Querier: Deprecate and enable by default `querier.ingester-metadata-streaming` flag. #6147
* [CHANGE] QueryFrontend/QueryScheduler: Deprecate `-querier.max-outstanding-requests-per-tenant` and `-query-scheduler.max-outstanding-requests-per-tenant` flags. Use frontend.max-outstanding-requests-per-tenant instead. #6146
* [CHANGE] Ingesters: Enable 'snappy-block' compression on ingester clients by default. #6148
* [CHANGE] Ruler: Scheduling `ruler.evaluation-delay-duration` to be deprecated. Ruler will use the highest value between `ruler.evaluation-delay-duration` and `ruler.query-offset` #6149
* [CHANGE] Querier: Remove `-querier.at-modifier-enabled` flag. #6157
* [CHANGE] Tracing: Remove deprecated `oltp_endpoint` config entirely. #6158
* [CHANGE] Store Gateway: Enable store gateway zone stable shuffle sharding by default. #6161
* [FEATURE] Ingester/Distributor: Experimental: Enable native histogram ingestion via `-blocks-storage.tsdb.enable-native-histograms` flag. #5986 #6010 #6020
* [FEATURE] Querier: Enable querying native histogram chunks. #5944 #6031
* [FEATURE] Query Frontend: Support native histogram in query frontend response. #5996 #6043
* [FEATURE] Ruler: Support sending native histogram samples to Ingester. #6029
* [FEATURE] Ruler: Add support for filtering out alerts in ListRules API. #6011
* [FEATURE] Query Frontend: Added a query rejection mechanism to block resource-intensive queries. #6005
* [FEATURE] OTLP: Support ingesting OTLP exponential metrics as native histograms. #6071 #6135
* [FEATURE] Ingester: Add `ingester.instance-limits.max-inflight-query-requests` to allow limiting ingester concurrent queries. #6081
* [FEATURE] Distributor: Add `validation.max-native-histogram-buckets` to limit max number of bucket count. Distributor will try to automatically reduce histogram resolution until it is within the bucket limit or resolution cannot be reduced anymore. #6104
* [FEATURE] Store Gateway: Introduce token bucket limiter to enhance store gateway throttling. #6016
* [FEATURE] Ruler: Add support for `query_offset` field on RuleGroup and new `ruler_query_offset` per-tenant limit. #6085
* [ENHANCEMENT] Ruler: Add support to persist tokens in rulers. #5987
* [ENHANCEMENT] Query Frontend/Querier: Added store gateway postings touched count and touched size in Querier stats and log in Query Frontend. #5892
* [ENHANCEMENT] Query Frontend/Querier: Returns `warnings` on prometheus query responses. #5916
* [ENHANCEMENT] Ingester: Allowing to configure `-blocks-storage.tsdb.head-compaction-interval` flag up to 30 min and add a jitter on the first head compaction. #5919 #5928
* [ENHANCEMENT] Distributor: Added `max_inflight_push_requests` config to ingester client to protect distributor from OOMKilled. #5917
* [ENHANCEMENT] Distributor/Querier: Clean stale per-ingester metrics after ingester restarts. #5930
* [ENHANCEMENT] Distributor/Ring: Allow disabling detailed ring metrics by ring member. #5931
* [ENHANCEMENT] KV: Etcd Added etcd.ping-without-stream-allowed parameter to disable/enable PermitWithoutStream #5933
* [ENHANCEMENT] Ingester: Add a new `limits_per_label_set` limit. This limit functions similarly to `max_series_per_metric`, but allowing users to define the maximum number of series per LabelSet. #5950 #5993
* [ENHANCEMENT] Store Gateway: Log gRPC requests together with headers configured in `http_request_headers_to_log`. #5958
* [ENHANCEMENT] Ingester: Add a new experimental `-ingester.labels-string-interning-enabled` flag to enable string interning for metrics labels. #6057
* [ENHANCEMENT] Ingester: Add link to renew 10% of the ingesters tokens in the admin page. #6063
* [ENHANCEMENT] Ruler: Add support for filtering by `state` and `health` field on Rules API. #6040
* [ENHANCEMENT] Ruler: Add support for filtering by `match` field on Rules API. #6083
* [ENHANCEMENT] Distributor: Reduce memory usage when error volume is high. #6095
* [ENHANCEMENT] Compactor: Centralize metrics used by compactor and add user label to compactor metrics. #6096
* [ENHANCEMENT] Compactor: Add unique execution ID for each compaction cycle in log for easy debugging. #6097
* [ENHANCEMENT] Compactor: Differentiate retry and halt error and retry failed compaction only on retriable error. #6111
* [ENHANCEMENT] Ruler: Add support for filtering by `state` and `health` field on Rules API. #6040
* [ENHANCEMENT] Compactor: Split cleaner cycle for active and deleted tenants. #6112
* [ENHANCEMENT] Compactor: Introduce cleaner visit marker. #6113
* [ENHANCEMENT] Query Frontend: Add `cortex_query_samples_total` metric. #6142
* [ENHANCEMENT] Ingester: Implement metadata API limit. #6128
* [BUGFIX] Configsdb: Fix endline issue in db password. #5920
* [BUGFIX] Ingester: Fix `user` and `type` labels for the `cortex_ingester_tsdb_head_samples_appended_total` TSDB metric. #5952
* [BUGFIX] Querier: Enforce max query length check for `/api/v1/series` API even though `ignoreMaxQueryLength` is set to true. #6018
* [BUGFIX] Ingester: Fix issue with the minimize token generator where it was not taking in consideration the current ownership of an instance when generating extra tokens. #6062
* [BUGFIX] Scheduler: Fix user queue in scheduler that was not thread-safe. #6077 #6160
* [BUGFIX] Ingester: Include out-of-order head compaction when compacting TSDB head. #6108
* [BUGFIX] Ingester: Fix `cortex_ingester_tsdb_mmap_chunks_total` metric. #6134
* [BUGFIX] Query Frontend: Fix query rejection bug for metadata queries. #6143

## 1.17.1 2024-05-20

* [CHANGE] Query Frontend/Ruler: Omit empty data, errorType and error fields in API response. #5953 #5954
* [ENHANCEMENT] Ingester: Added `upload_compacted_blocks_enabled` config to ingester to parameterize uploading compacted blocks. #5959
* [BUGFIX] Querier: Select correct tenant during query federation. #5943

## 1.17.0 2024-04-30

* [CHANGE] Azure Storage: Upgraded objstore dependency and support Azure Workload Identity Authentication. Added `connection_string` to support authenticating via SAS token. Marked `msi_resource` config as deprecating. #5645
* [CHANGE] Store Gateway: Add a new fastcache based inmemory index cache. #5619
* [CHANGE] Index Cache: Multi level cache backfilling operation becomes async. Added `-blocks-storage.bucket-store.index-cache.multilevel.max-async-concurrency` and `-blocks-storage.bucket-store.index-cache.multilevel.max-async-buffer-size` configs and metric `cortex_store_multilevel_index_cache_backfill_dropped_items_total` for number of dropped items. #5661
* [CHANGE] Ingester: Disable uploading compacted blocks and overlapping compaction in ingester. #5735
* [CHANGE] Distributor: Count the number of rate-limited samples in `distributor_samples_in_total`. #5714
* [CHANGE] Ruler: Remove `cortex_ruler_write_requests_total`, `cortex_ruler_write_requests_failed_total`, `cortex_ruler_queries_total`, `cortex_ruler_queries_failed_total`, and `cortex_ruler_query_seconds_total` metrics for the tenant when the ruler deletes the manager for the tenant. #5772
* [CHANGE] Main: Mark `mem-ballast-size-bytes` flag as deprecated. #5816
* [CHANGE] Querier: Mark `-querier.ingester-streaming` flag as deprecated. Now query ingester streaming is always enabled. #5817
* [CHANGE] Compactor/Bucket Store: Added `-blocks-storage.bucket-store.block-discovery-strategy` to configure different block listing strategy. Reverted the current recursive block listing mechanism and use the strategy `Concurrent` as in 1.15. #5828
* [CHANGE] Compactor: Don't halt compactor when overlapped source blocks detected. #5854
* [CHANGE] S3 Bucket Client: Expose `-blocks-storage.s3.send-content-md5` flag and set default checksum algorithm to MD5. #5870
* [CHANGE] Querier: Mark `querier.iterators` and `querier.batch-iterators` flags as deprecated. Now querier always use batch iterators. #5868
* [CHANGE] Query Frontend: Error response returned by Query Frontend now follows Prometheus API error response format. #5811
* [FEATURE] Experimental: OTLP ingestion. #5813
* [FEATURE] Query Frontend/Scheduler: Add query priority support. #5605
* [FEATURE] Tracing: Use `kuberesolver` to resolve OTLP endpoints address with `kubernetes://` prefix as Kubernetes service. #5731
* [FEATURE] Tracing: Add `tracing.otel.round-robin` flag to use `round_robin` gRPC client side LB policy for sending OTLP traces. #5731
* [FEATURE] Ruler: Add `ruler.concurrent-evals-enabled` flag to enable concurrent evaluation within a single rule group for independent rules. Maximum concurrency can be configured via `ruler.max-concurrent-evals`. #5766
* [FEATURE] Distributor Queryable: Experimental: Add config `zone_results_quorum_metadata`. When querying ingesters using metadata APIs such as label names and values, only results from quorum number of zones will be included and merged. #5779
* [FEATURE] Storage Cache Clients: Add config `set_async_circuit_breaker_config` to utilize the circuit breaker pattern for dynamically thresholding asynchronous set operations. Implemented in both memcached and redis cache clients. #5789
* [FEATURE] Ruler: Add experimental `experimental.ruler.api-deduplicate-rules` flag to remove duplicate rule groups from the Prometheus compatible rules API endpoint. Add experimental `ruler.ring.replication-factor` and `ruler.ring.zone-awareness-enabled` flags to configure rule group replication, but only the first ruler in the replicaset evaluates the rule group, the rest will be used as backup to handle events when a ruler is down during an API request to list rules. #5782 #5901
* [FEATURE] Ring: Add experimental `-ingester.tokens-generator-strategy=minimize-spread` flag to enable the new minimize spread token generator strategy. #5855
* [FEATURE] Ring Status Page: Add `Ownership Diff From Expected` column in the ring table to indicate the extent to which the ownership of a specific ingester differs from the expected ownership. #5889
* [ENHANCEMENT] Ingester: Add per-tenant new metric `cortex_ingester_tsdb_data_replay_duration_seconds`. #5477
* [ENHANCEMENT] Store Gateway: Added `-store-gateway.enabled-tenants` and `-store-gateway.disabled-tenants` to explicitly enable or disable store-gateway for specific tenants. #5638
* [ENHANCEMENT] Query Frontend: Write service timing header in response even though there is an error. #5653
* [ENHANCEMENT] Compactor: Add new compactor metric `cortex_compactor_start_duration_seconds`. #5683
* [ENHANCEMENT] Index Cache: Multi level cache adds config `max_backfill_items` to cap max items to backfill per async operation. #5686
* [ENHANCEMENT] Query Frontend: Log number of split queries in `query stats` log. #5703
* [ENHANCEMENT] Compactor: Skip compaction retry when encountering a permission denied error. #5727
* [ENHANCEMENT] Logging: Added new options for logging HTTP request headers: `-server.log-request-headers` enables logging HTTP request headers, `-server.log-request-headers-exclude-list` allows users to specify headers which should not be logged. #5744
* [ENHANCEMENT] Query Frontend/Scheduler: Time check in query priority now considers overall data select time window (including range selectors, modifiers and lookback delta). #5758
* [ENHANCEMENT] Querier: Added `querier.store-gateway-query-stats-enabled` to enable or disable store gateway query stats log. #5749
* [ENHANCEMENT] Querier: Improve labels APIs latency by merging slices using K-way merge and more than 1 core. #5785
* [ENHANCEMENT] AlertManager: Retrying AlertManager Delete Silence on error. #5794
* [ENHANCEMENT] Ingester: Add new ingester metric `cortex_ingester_max_inflight_query_requests`. #5798
* [ENHANCEMENT] Query: Added `query_storage_wall_time` to Query Frontend and Ruler query stats log for wall time spent on fetching data from storage. Query evaluation is not included. #5799
* [ENHANCEMENT] Query: Added additional max query length check at Query Frontend and Ruler. Added `-querier.ignore-max-query-length` flag to disable max query length check at Querier. #5808
* [ENHANCEMENT] Querier: Add context error check when converting Metrics to SeriesSet for GetSeries on distributorQuerier. #5827
* [ENHANCEMENT] Ruler: Improve GetRules response time by reducing lock contention and introducing a temporary rules cache in `ruler/manager.go`. #5805
* [ENHANCEMENT] Querier: Add context error check when merging slices from ingesters for GetLabel operations. #5837
* [BUGFIX] Distributor: Do not use label with empty values for sharding #5717
* [BUGFIX] Query Frontend: queries with negative offset should check whether it is cacheable or not. #5719
* [BUGFIX] Redis Cache: pass `cache_size` config correctly. #5734
* [BUGFIX] Distributor: Shuffle-Sharding with `ingestion_tenant_shard_size` set to 0, default sharding strategy should be used. #5189
* [BUGFIX] Cortex: Fix GRPC stream clients not honoring overrides for call options. #5797
* [BUGFIX] Ruler: Fix support for `keep_firing_for` field in alert rules. #5823
* [BUGFIX] Ring DDB: Fix lifecycle for ring counting unhealthy pods as healthy. #5838
* [BUGFIX] Ring DDB: Fix region assignment. #5842

## 1.16.1 2024-04-23

* [ENHANCEMENT] Upgraded Docker base images to `alpine:3.18`. #5684
* [ENHANCEMENT] Upgrade to go 1.21.9 #5879 #5882

## 1.16.0 2023-11-20

* [CHANGE] AlertManager: include reason label in `cortex_alertmanager_notifications_failed_total`. #5409
* [CHANGE] Ruler: Added user label to `cortex_ruler_write_requests_total`, `cortex_ruler_write_requests_failed_total`, `cortex_ruler_queries_total`, and `cortex_ruler_queries_failed_total` metrics. #5312
* [CHANGE] Alertmanager: Validating new fields on the PagerDuty AM config. #5290
* [CHANGE] Ingester: Creating label `native-histogram-sample` on the `cortex_discarded_samples_total` to keep track of discarded native histogram samples. #5289
* [CHANGE] Store Gateway: Rename `cortex_bucket_store_cached_postings_compression_time_seconds` to `cortex_bucket_store_cached_postings_compression_time_seconds_total`. #5431
* [CHANGE] Store Gateway: Rename `cortex_bucket_store_cached_series_fetch_duration_seconds` to `cortex_bucket_store_series_fetch_duration_seconds` and `cortex_bucket_store_cached_postings_fetch_duration_seconds` to `cortex_bucket_store_postings_fetch_duration_seconds`. Add new metric `cortex_bucket_store_chunks_fetch_duration_seconds`. #5448
* [CHANGE] Store Gateway: Remove `idle_timeout`, `max_conn_age`, `pool_size`, `min_idle_conns` fields for Redis index cache and caching bucket. #5448
* [CHANGE] Store Gateway: Add flag `-store-gateway.sharding-ring.zone-stable-shuffle-sharding` to enable store gateway to use zone stable shuffle sharding. #5489
* [CHANGE] Bucket Index: Add `series_max_size` and `chunk_max_size` to bucket index. #5489
* [CHANGE] StoreGateway: Rename `cortex_bucket_store_chunk_pool_returned_bytes_total` and `cortex_bucket_store_chunk_pool_requested_bytes_total` to `cortex_bucket_store_chunk_pool_operation_bytes_total`. #5552
* [CHANGE] Query Frontend/Querier: Make build info API disabled by default and add feature flag `api.build-info-enabled` to enable it. #5533
* [CHANGE] Purger: Do no use S3 tenant kms key when uploading deletion marker. #5575
* [CHANGE] Ingester: Shipper always allows uploading compacted blocks to ship OOO compacted blocks. #5625
* [CHANGE] DDBKV: Change metric name from `dynamodb_kv_read_capacity_total` to `dynamodb_kv_consumed_capacity_total` and include Delete, Put, Batch dimension. #5487
* [CHANGE] Compactor: Adding the userId on the compact dir path. #5524
* [CHANGE] Ingester: Remove deprecated ingester metrics. #5472
* [CHANGE] Query Frontend: Expose `-querier.max-subquery-steps` to configure subquery max steps check. By default, the limit is set to 0, which is disabled. #5656
* [FEATURE] Store Gateway: Implementing multi level index cache. #5451
* [FEATURE] Ruler: Add support for disabling rule groups. #5521
* [FEATURE] Support object storage backends for runtime configuration file. #5292
* [FEATURE] Ruler: Add support for `Limit` field on RuleGroup. #5528
* [FEATURE] AlertManager: Add support for Webex, Discord and Telegram Receiver. #5493
* [FEATURE] Ingester: added `-admin-limit-message` to customize the message contained in limit errors.#5460
* [FEATURE] AlertManager: Update version to v0.26.0 and bring in Microsoft Teams receiver. #5543
* [FEATURE] Store Gateway: Support lazy expanded posting optimization. Added new flag `blocks-storage.bucket-store.lazy-expanded-postings-enabled` and new metrics `cortex_bucket_store_lazy_expanded_postings_total`, `cortex_bucket_store_lazy_expanded_posting_size_bytes_total` and `cortex_bucket_store_lazy_expanded_posting_series_overfetched_size_bytes_total`. #5556.
* [FEATURE] Store Gateway: Add `max_downloaded_bytes_per_request` to limit max bytes to download per store gateway request. #5179
* [FEATURE] Added 2 flags `-alertmanager.alertmanager-client.grpc-max-send-msg-size` and ` -alertmanager.alertmanager-client.grpc-max-recv-msg-size` to configure alert manager grpc client message size limits. #5338
* [FEATURE] Querier/StoreGateway: Allow the tenant shard sizes to be a percent of total instances. #5393
* [FEATURE] Added the flag `-alertmanager.api-concurrency` to configure alert manager api concurrency limit. #5412
* [FEATURE] Store Gateway: Add `-store-gateway.sharding-ring.keep-instance-in-the-ring-on-shutdown` to skip unregistering instance from the ring in shutdown. #5421
* [FEATURE] Ruler: Support for filtering rules in the API. #5417
* [FEATURE] Compactor: Add `-compactor.ring.tokens-file-path` to store generated tokens locally. #5432
* [FEATURE] Query Frontend: Add `-frontend.retry-on-too-many-outstanding-requests` to re-enqueue 429 requests if there are multiple query-schedulers available. #5496
* [FEATURE] Store Gateway: Add `-blocks-storage.bucket-store.max-inflight-requests` for store gateways to reject further series requests upon reaching the limit. #5553
* [FEATURE] Store Gateway: Support filtered index cache. #5587
* [ENHANCEMENT] Update go version to 1.21.3. #5630
* [ENHANCEMENT] Store Gateway: Add `cortex_bucket_store_block_load_duration_seconds` histogram to track time to load blocks. #5580
* [ENHANCEMENT] Querier: retry chunk pool exhaustion error in querier rather than query frontend. #5569
* [ENHANCEMENT] Alertmanager: Added flag `-alertmanager.alerts-gc-interval` to configure alerts Garbage collection interval. #5550
* [ENHANCEMENT] Query Frontend: enable vertical sharding on binary expr . #5507
* [ENHANCEMENT] Query Frontend: Include user agent as part of query frontend log. #5450
* [ENHANCEMENT] Query: Set CORS Origin headers for Query API #5388
* [ENHANCEMENT] Query Frontend: Add `cortex_rejected_queries_total` metric for throttled queries. #5356
* [ENHANCEMENT] Query Frontend: Optimize the decoding of `SampleStream`. #5349
* [ENHANCEMENT] Compactor: Check ctx done when uploading visit marker. #5333
* [ENHANCEMENT] AlertManager: Add `cortex_alertmanager_dispatcher_aggregation_groups` and `cortex_alertmanager_dispatcher_alert_processing_duration_seconds` metrics for dispatcher. #5592
* [ENHANCEMENT] Store Gateway: Added new flag `blocks-storage.bucket-store.series-batch-size` to control how many series to fetch per batch in Store Gateway. #5582.
* [ENHANCEMENT] Querier: Log query stats when querying store gateway. #5376
* [ENHANCEMENT] Ruler: Add `cortex_ruler_rule_group_load_duration_seconds` and `cortex_ruler_rule_group_sync_duration_seconds` metrics. #5609
* [ENHANCEMENT] Ruler: Add contextual info and query statistics to log #5604
* [ENHANCEMENT] Distributor/Ingester: Add span on push path #5319
* [ENHANCEMENT] Query Frontend: Reject subquery with too small step size. #5323
* [ENHANCEMENT] Compactor: Exposing Thanos `accept-malformed-index` to Cortex compactor. #5334
* [ENHANCEMENT] Log: Avoid expensive `log.Valuer` evaluation for disallowed levels. #5297
* [ENHANCEMENT] Improving Performance on the API Gzip Handler. #5347
* [ENHANCEMENT] Dynamodb: Add `puller-sync-time` to allow different pull time for ring. #5357
* [ENHANCEMENT] Emit querier `max_concurrent` as a metric. #5362
* [ENHANCEMENT] Avoid sort tokens on lifecycler autoJoin. #5394
* [ENHANCEMENT] Do not resync blocks in running store gateways during rollout deployment and container restart. #5363
* [ENHANCEMENT] Store Gateway: Add new metrics `cortex_bucket_store_sent_chunk_size_bytes`, `cortex_bucket_store_postings_size_bytes` and `cortex_bucket_store_empty_postings_total`. #5397
* [ENHANCEMENT] Add jitter to lifecycler heartbeat. #5404
* [ENHANCEMENT] Store Gateway: Add config `estimated_max_series_size_bytes` and `estimated_max_chunk_size_bytes` to address data overfetch. #5401
* [ENHANCEMENT] Distributor/Ingester: Add experimental `-distributor.sign_write_requests` flag to sign the write requests. #5430
* [ENHANCEMENT] Store Gateway/Querier/Compactor: Handling CMK Access Denied errors. #5420 #5442 #5446
* [ENHANCEMENT] Alertmanager: Add the alert name in error log when it get throttled. #5456
* [ENHANCEMENT] Querier: Retry store gateway on different zones when zone awareness is enabled. #5476
* [ENHANCEMENT] Compactor: allow `unregister_on_shutdown` to be configurable. #5503
* [ENHANCEMENT] Querier: Batch adding series to query limiter to optimize locking. #5505
* [ENHANCEMENT] Store Gateway: add metric `cortex_bucket_store_chunk_refetches_total` for number of chunk refetches. #5532
* [ENHANCEMENT] BasicLifeCycler: allow final-sleep during shutdown #5517
* [ENHANCEMENT] All: Handling CMK Access Denied errors. #5420 #5542
* [ENHANCEMENT] Querier: Retry store gateway client connection closing gRPC error. #5558
* [ENHANCEMENT] QueryFrontend: Add generic retry for all APIs. #5561.
* [ENHANCEMENT] Querier: Check context before notifying scheduler and frontend. #5565
* [ENHANCEMENT] QueryFrontend: Add metric for number of series requests. #5373
* [ENHANCEMENT] Store Gateway: Add histogram metrics for total time spent fetching series and chunks per request. #5573
* [ENHANCEMENT] Store Gateway: Check context in multi level cache. Add `cortex_store_multilevel_index_cache_fetch_duration_seconds` and `cortex_store_multilevel_index_cache_backfill_duration_seconds` to measure fetch and backfill latency. #5596
* [ENHANCEMENT] Ingester: Added new ingester TSDB metrics `cortex_ingester_tsdb_head_samples_appended_total`, `cortex_ingester_tsdb_head_out_of_order_samples_appended_total`, `cortex_ingester_tsdb_snapshot_replay_error_total`, `cortex_ingester_tsdb_sample_ooo_delta` and `cortex_ingester_tsdb_mmap_chunks_total`. #5624
* [ENHANCEMENT] Query Frontend: Handle context error before decoding and merging responses. #5499
* [ENHANCEMENT] Store-Gateway and AlertManager: Add a `wait_instance_time_out` to context to avoid waiting forever. #5581
* [ENHANCEMENT] Blocks storage: Move the tenant deletion mark from `<tenantID>/markers/tenant-deletion-mark.json` to  `__markers__/<tenantID>/tenant-deletion-mark.json`. #5676
* [BUGFIX] Compactor: Fix possible division by zero during compactor config validation. #5535
* [BUGFIX] Ruler: Validate if rule group can be safely converted back to rule group yaml from protobuf message #5265
* [BUGFIX] Querier: Convert gRPC `ResourceExhausted` status code from store gateway to 422 limit error. #5286
* [BUGFIX] Alertmanager: Route web-ui requests to the alertmanager distributor when sharding is enabled. #5293
* [BUGFIX] Storage: Bucket index updater should ignore meta not found for partial blocks. #5343
* [BUGFIX] Ring: Add `JOINING` state to read operation. #5346
* [BUGFIX] Compactor: Partial block with only visit marker should be deleted even there is no deletion marker. #5342
* [BUGFIX] KV: Etcd calls will no longer block indefinitely and will now time out after the `DialTimeout` period. #5392
* [BUGFIX] Ring: Allow RF greater than number of zones to select more than one instance per zone #5411
* [BUGFIX] Store Gateway: Fix bug in store gateway ring comparison logic. #5426
* [BUGFIX] Ring: Fix bug in consistency of Get func in a scaling zone-aware ring. #5429
* [BUGFIX] Compactor: Fix retry on markers. #5441
* [BUGFIX] Query Frontend: Fix bug of failing to cancel downstream request context in query frontend v2 mode (query scheduler enabled). #5447
* [BUGFIX] Alertmanager: Remove the user id from state replication key metric label value. #5453
* [BUGFIX] Compactor: Avoid cleaner concurrency issues checking global markers before all blocks. #5457
* [BUGFIX] DDBKV: Disallow instance with older timestamp to update instance with newer timestamp. #5480
* [BUGFIX] DDBKV: When no change detected in ring, retry the CAS until there is change. #5502
* [BUGFIX] Fix bug on objstore when configured to use S3 fips endpoints. #5540
* [BUGFIX] Ruler: Fix bug on ruler where a failure to load a single RuleGroup would prevent rulers to sync all RuleGroup. #5563
* [BUGFIX] Query Frontend: Fix query string being omitted in query stats log. #5655

## 1.15.3 2023-06-22

* [BUGFIX] Distributor: Fix potential data corruption in cases of timeout between distributors and ingesters. #5422

## 1.15.2 2023-05-09

* [ENHANCEMENT] Update Go version to 1.20.4. #5299

## 1.15.1 2023-04-26

* [CHANGE] Alertmanager: Validating new fields on the PagerDuty AM config. #5290
* [BUGFIX] Querier: Convert gRPC `ResourceExhausted` status code from store gateway to 422 limit error. #5286

## 1.15.0 2023-04-19

* [CHANGE] Storage: Make Max exemplars config per tenant instead of global configuration. #5080 #5122
* [CHANGE] Alertmanager: Local file disclosure vulnerability in OpsGenie configuration has been fixed. #5045
* [CHANGE] Rename oltp_endpoint to otlp_endpoint to match opentelemetry spec and lib name. #5068
* [CHANGE] Distributor/Ingester: Log warn level on push requests when they have status code 4xx. Do not log if status is 429. #5103
* [CHANGE] Tracing: Use the default OTEL trace sampler when `-tracing.otel.exporter-type` is set to `awsxray`. #5141
* [CHANGE] Ingester partial error log line to debug level. #5192
* [CHANGE] Change HTTP status code from 503/422 to 499 if a request is canceled. #5220
* [CHANGE] Store gateways summary metrics have been converted to histograms `cortex_bucket_store_series_blocks_queried`, `cortex_bucket_store_series_data_fetched`, `cortex_bucket_store_series_data_size_touched_bytes`, `cortex_bucket_store_series_data_size_fetched_bytes`, `cortex_bucket_store_series_data_touched`, `cortex_bucket_store_series_result_series` #5239
* [FEATURE] Querier/Query Frontend: support Prometheus /api/v1/status/buildinfo API. #4978
* [FEATURE] Ingester: Add active series to all_user_stats page. #4972
* [FEATURE] Ingester: Added `-blocks-storage.tsdb.head-chunks-write-queue-size` allowing to configure the size of the in-memory queue used before flushing chunks to the disk . #5000
* [FEATURE] Query Frontend: Log query params in query frontend even if error happens. #5005
* [FEATURE] Ingester: Enable snapshotting of In-memory TSDB on disk during shutdown via `-blocks-storage.tsdb.memory-snapshot-on-shutdown`. #5011
* [FEATURE] Query Frontend/Scheduler: Add a new counter metric `cortex_request_queue_requests_total` for total requests going to queue. #5030
* [FEATURE] Build ARM docker images. #5041
* [FEATURE] Query-frontend/Querier: Create spans to measure time to merge promql responses. #5041
* [FEATURE] Querier/Ruler: Support the new thanos promql engine. This is an experimental feature and might change in the future. #5093
* [FEATURE] Added zstd as an option for grpc compression #5092
* [FEATURE] Ring: Add new kv store option `dynamodb`. #5026
* [FEATURE] Cache: Support redis as backend for caching bucket and index cache. #5057
* [FEATURE] Querier/Store-Gateway: Added `-blocks-storage.bucket-store.ignore-blocks-within` allowing to filter out the recently created blocks from being synced by queriers and store-gateways. #5166
* [FEATURE] AlertManager/Ruler: Added support for  `keep_firing_for` on alerting rulers.
* [FEATURE] Alertmanager: Add support for time_intervals. #5102
* [FEATURE] Added `snappy-block` as an option for grpc compression #5215
* [FEATURE] Enable experimental out-of-order samples support. Added 2 new configs `ingester.out_of_order_time_window` and `blocks-storage.tsdb.out_of_order_cap_max`. #4964
* [ENHANCEMENT] Querier: limit series query to only ingesters if `start` param is not specified. #4976
* [ENHANCEMENT] Query-frontend/scheduler: add a new limit `frontend.max-outstanding-requests-per-tenant` for configuring queue size per tenant. Started deprecating two flags `-query-scheduler.max-outstanding-requests-per-tenant` and `-querier.max-outstanding-requests-per-tenant`, and change their value default to 0. Now if both the old flag and new flag are specified, the old flag's queue size will be picked. #4991
* [ENHANCEMENT] Query-tee: Add `/api/v1/query_exemplars` API endpoint support. #5010
* [ENHANCEMENT] Let blocks_cleaner delete blocks concurrently(default 16 goroutines). #5028
* [ENHANCEMENT] Query Frontend/Query Scheduler: Increase upper bound to 60s for queue duration histogram metric. #5029
* [ENHANCEMENT] Query Frontend: Log Vertical sharding information when `query_stats_enabled` is enabled. #5037
* [ENHANCEMENT] Ingester: The metadata APIs should honour `querier.query-ingesters-within` when `querier.query-store-for-labels-enabled` is true. #5027
* [ENHANCEMENT] Query Frontend: Skip instant query roundtripper if sharding is not applicable. #5062
* [ENHANCEMENT] Push reduce one hash operation of Labels. #4945 #5114
* [ENHANCEMENT] Alertmanager: Added `-alertmanager.enabled-tenants` and `-alertmanager.disabled-tenants` to explicitly enable or disable alertmanager for specific tenants. #5116
* [ENHANCEMENT] Upgraded Docker base images to `alpine:3.17`. #5132
* [ENHANCEMENT] Add retry logic to S3 bucket client. #5135
* [ENHANCEMENT] Update Go version to 1.20.1. #5159
* [ENHANCEMENT] Distributor: Reuse byte slices when serializing requests from distributors to ingesters. #5193
* [ENHANCEMENT] Query Frontend: Add number of chunks and samples fetched in query stats. #5198
* [ENHANCEMENT] Implement grpc.Compressor.DecompressedSize for snappy to optimize memory allocations. #5213
* [ENHANCEMENT] Querier: Batch Iterator optimization to prevent transversing it multiple times query ranges steps does not overlap. #5237
* [BUGFIX] Updated `golang.org/x/net` dependency to fix CVE-2022-27664. #5008
* [BUGFIX] Fix panic when otel and xray tracing is enabled. #5044
* [BUGFIX] Fixed no compact block got grouped in shuffle sharding grouper. #5055
* [BUGFIX] Fixed ingesters with less tokens stuck in LEAVING. #5061
* [BUGFIX] Tracing: Fix missing object storage span instrumentation. #5074
* [BUGFIX] Ingester: Fix Ingesters returning empty response for metadata APIs. #5081
* [BUGFIX] Ingester: Fix panic when querying metadata from blocks that are being deleted. #5119
* [BUGFIX] Ring: Fix case when dynamodb kv reaches the limit of 25 actions per batch call. #5136
* [BUGFIX] Query-frontend: Fix shardable instant queries do not produce sorted results for `sort`, `sort_desc`, `topk`, `bottomk` functions. #5148, #5170
* [BUGFIX] Querier: Fix `/api/v1/series` returning 5XX instead of 4XX when limits are hit. #5169
* [BUGFIX] Compactor: Fix issue that shuffle sharding planner return error if block is under visit by other compactor. #5188
* [BUGFIX] Fix S3 BucketWithRetries upload empty content issue #5217
* [BUGFIX] Query Frontend: Disable `absent`, `absent_over_time` and `scalar` for vertical sharding. #5221
* [BUGFIX] Catch context error in the s3 bucket client. #5240
* [BUGFIX] Fix query frontend remote read empty body. #5257
* [BUGFIX] Fix query frontend incorrect error response format at `SplitByQuery` middleware. #5260

## 1.14.0 2022-12-02

  **This release removes support for chunks storage. See below for more.**
* [CHANGE] Remove support for chunks storage entirely. If you are using chunks storage on a previous version, you must [migrate your data](https://github.com/cortexproject/cortex/blob/v1.13.1/docs/blocks-storage/migrate-from-chunks-to-blocks.md) on version 1.13.1 or earlier. Before upgrading to this release, you should also remove any deprecated chunks-related configuration, as this release will no longer accept that. The following flags are gone:
  - `-dynamodb.*`
  - `-metrics.*`
  - `-s3.*`
  - `-azure.*`
  - `-bigtable.*`
  - `-gcs.*`
  - `-cassandra.*`
  - `-boltdb.*`
  - `-local.*`
  - some `-ingester` flags:
    - `-ingester.wal-enabled`
    - `-ingester.checkpoint-enabled`
    - `-ingester.recover-from-wal`
    - `-ingester.wal-dir`
    - `-ingester.checkpoint-duration`
    - `-ingester.flush-on-shutdown-with-wal-enabled`
    - `-ingester.max-transfer-retries`
    - `-ingester.max-samples-per-query`
    - `-ingester.min-chunk-length`
    - `-ingester.flush-period`
    - `-ingester.retain-period`
    - `-ingester.max-chunk-idle`
    - `-ingester.max-stale-chunk-idle`
    - `-ingester.flush-op-timeout`
    - `-ingester.max-chunk-age`
    - `-ingester.chunk-age-jitter`
    - `-ingester.concurrent-flushes`
    - `-ingester.spread-flushes`
    - `-ingester.chunk-encoding`
    - `-store.*` except `-store.engine` and `-store.max-query-length`
    - `-store.query-chunk-limit` was deprecated and replaced by `-querier.max-fetched-chunks-per-query`
  - `-deletes.*`
  - `-grpc-store.*`
  - `-flusher.wal-dir`, `-flusher.concurrent-flushes`, `-flusher.flush-op-timeout`
* [CHANGE] Remove support for alertmanager and ruler legacy store configuration. Before upgrading, you need to convert your configuration to use the `alertmanager-storage` and `ruler-storage` configuration on the version that you're already running, then upgrade.
* [CHANGE] Disables TSDB isolation. #4825
* [CHANGE] Drops support Prometheus 1.x rule format on configdb. #4826
* [CHANGE] Removes `-ingester.stream-chunks-when-using-blocks` experimental flag and stream chunks by default when `querier.ingester-streaming` is enabled. #4864
* [CHANGE] Compactor: Added `cortex_compactor_runs_interrupted_total` to separate compaction interruptions from failures
* [CHANGE] Enable PromQL `@` modifier, negative offset always. #4927
* [CHANGE] Store-gateway: Add user label to `cortex_bucket_store_blocks_loaded` metric. #4918
* [CHANGE] AlertManager: include `status` label in `cortex_alertmanager_alerts_received_total`. #4907
* [FEATURE] Compactor: Added `-compactor.block-files-concurrency` allowing to configure number of go routines for download/upload block files during compaction. #4784
* [FEATURE] Compactor: Added `-compactor.blocks-fetch-concurrency` allowing to configure number of go routines for blocks during compaction. #4787
* [FEATURE] Compactor: Added configurations for Azure MSI in blocks-storage, ruler-storage and alertmanager-storage. #4818
* [FEATURE] Ruler: Add support to pass custom implementations of queryable and pusher. #4782
* [FEATURE] Create OpenTelemetry Bridge for Tracing. Now cortex can send traces to multiple destinations using OTEL Collectors. #4834
* [FEATURE] Added `-api.http-request-headers-to-log` allowing for the addition of HTTP Headers to logs #4803
* [FEATURE] Distributor: Added a new limit `-validation.max-labels-size-bytes` allowing to limit the combined size of labels for each timeseries. #4848
* [FEATURE] Storage/Bucket: Added `-*.s3.bucket-lookup-type` allowing to configure the s3 bucket lookup type. #4794
* [FEATURE] QueryFrontend: Implement experimental vertical sharding at query frontend for range/instant queries. #4863
* [FEATURE] QueryFrontend: Support vertical sharding for subqueries. #4955
* [FEATURE] Querier: Added a new limit `-querier.max-fetched-data-bytes-per-query` allowing to limit the maximum size of all data in bytes that a query can fetch from each ingester and storage. #4854
* [FEATURE] Added 2 flags `-alertmanager.alertmanager-client.grpc-compression` and `-querier.store-gateway-client.grpc-compression` to configure compression methods for grpc clients. #4889
* [ENHANCEMENT] AlertManager: Retrying AlertManager Get Requests (Get Alertmanager status, Get Alertmanager Receivers) on next replica on error #4840
* [ENHANCEMENT] Querier/Ruler: Retry store-gateway in case of unexpected failure, instead of failing the query. #4532 #4839
* [ENHANCEMENT] Ring: DoBatch prioritize 4xx errors when failing. #4783
* [ENHANCEMENT] Cortex now built with Go 1.18. #4829
* [ENHANCEMENT] Ingester: Prevent ingesters to become unhealthy during wall replay. #4847
* [ENHANCEMENT] Compactor: Introduced visit marker file for blocks so blocks are under compaction will not be picked up by another compactor. #4805
* [ENHANCEMENT] Distributor: Add label name to labelValueTooLongError. #4855
* [ENHANCEMENT] Enhance traces with hostname information. #4898
* [ENHANCEMENT] Improve the documentation around limits. #4905
* [ENHANCEMENT] Distributor: cache user overrides to reduce lock contention. #4904
* [BUGFIX] Storage/Bucket: Enable AWS SDK for go authentication for s3 to fix IMDSv1 authentication. #4897
* [BUGFIX] Memberlist: Add join with no retrying when starting service. #4804
* [BUGFIX] Ruler: Fix /ruler/rule_groups returns YAML with extra fields. #4767
* [BUGFIX] Respecting `-tracing.otel.sample-ratio` configuration when enabling OpenTelemetry tracing with X-ray. #4862
* [BUGFIX] QueryFrontend: fixed query_range requests when query has `start` equals to `end`. #4877
* [BUGFIX] AlertManager: fixed issue introduced by #4495 where templates files were being deleted when using alertmanager local store. #4890
* [BUGFIX] Ingester: fixed incorrect logging at the start of ingester block shipping logic. #4934
* [BUGFIX] Storage/Bucket: fixed global mark missing on deletion. #4949
* [BUGFIX] QueryFrontend/Querier: fixed regression added by #4863 where we stopped compressing the response between querier and query frontend. #4960
* [BUGFIX] QueryFrontend/Querier: fixed fix response error to be ungzipped when status code is not 2xx. #4975

### Known issues

- Configsdb: Ruler configs doesn't work. Remove all configs from postgres database that have format Prometheus 1.x rule format before upgrading to v1.14.0 (see [5387](https://github.com/cortexproject/cortex/issues/5387))

## 1.13.0 2022-07-14

* [CHANGE] Changed default for `-ingester.min-ready-duration` from 1 minute to 15 seconds. #4539
* [CHANGE] query-frontend: Do not print anything in the logs of `query-frontend` if a in-progress query has been canceled (context canceled) to avoid spam. #4562
* [CHANGE] Compactor block deletion mark migration, needed when upgrading from v1.7, is now disabled by default. #4597
* [CHANGE] The `status_code` label on gRPC client metrics has changed from '200' and '500' to '2xx', '5xx', '4xx', 'cancel' or 'error'. #4601
* [CHANGE] Memberlist: changed probe interval from `1s` to `5s` and probe timeout from `500ms` to `2s`. #4601
* [CHANGE] Fix incorrectly named `cortex_cache_fetched_keys` and `cortex_cache_hits` metrics. Renamed to `cortex_cache_fetched_keys_total` and `cortex_cache_hits_total` respectively. #4686
* [CHANGE] Enable Thanos series limiter in store-gateway. #4702
* [CHANGE] Distributor: Apply `max_fetched_series_per_query` limit for `/series` API. #4683
* [CHANGE] Re-enable the `proxy_url` option for receiver configuration. #4741
* [FEATURE] Ruler: Add `external_labels` option to tag all alerts with a given set of labels. #4499
* [FEATURE] Compactor: Add `-compactor.skip-blocks-with-out-of-order-chunks-enabled` configuration to mark blocks containing index with out-of-order chunks for no compact instead of halting the compaction. #4707
* [FEATURE] Querier/Query-Frontend: Add `-querier.per-step-stats-enabled` and `-frontend.cache-queryable-samples-stats` configurations to enable query sample statistics. #4708
* [FEATURE] Add shuffle sharding for the compactor #4433
* [FEATURE] Querier: Use streaming for ingester metadata APIs. #4725
* [ENHANCEMENT] Update Go version to 1.17.8. #4602 #4604 #4658
* [ENHANCEMENT] Keep track of discarded samples due to bad relabel configuration in `cortex_discarded_samples_total`. #4503
* [ENHANCEMENT] Ruler: Add `-ruler.disable-rule-group-label` to disable the `rule_group` label on exported metrics. #4571
* [ENHANCEMENT] Query federation: improve performance in MergeQueryable by memoizing labels. #4502
* [ENHANCEMENT] Added new ring related config `-ingester.readiness-check-ring-health` when enabled the readiness probe will succeed only after all instances are ACTIVE and healthy in the ring, this is enabled by default. #4539
* [ENHANCEMENT] Added new ring related config `-distributor.excluded-zones` when set this will exclude the comma-separated zones from the ring, default is "". #4539
* [ENHANCEMENT] Upgraded Docker base images to `alpine:3.14`. #4514
* [ENHANCEMENT] Updated Prometheus to latest. Includes changes from prometheus#9239, adding 15 new functions. Multiple TSDB bugfixes prometheus#9438 & prometheus#9381. #4524
* [ENHANCEMENT] Query Frontend: Add setting `-frontend.forward-headers-list` in frontend  to configure the set of headers from the requests to be forwarded to downstream requests. #4486
* [ENHANCEMENT] Blocks storage: Add `-blocks-storage.azure.http.*`, `-alertmanager-storage.azure.http.*`, and `-ruler-storage.azure.http.*` to configure the Azure storage client. #4581
* [ENHANCEMENT] Optimise memberlist receive path when used as a backing store for rings with a large number of members. #4601
* [ENHANCEMENT] Add length and limit to labelNameTooLongError and labelValueTooLongError #4595
* [ENHANCEMENT] Add jitter to rejoinInterval. #4747
* [ENHANCEMENT] Compactor: uploading blocks no compaction marks to the global location and introduce a new metric #4729
  * `cortex_bucket_blocks_marked_for_no_compaction_count`: Total number of blocks marked for no compaction in the bucket.
* [ENHANCEMENT] Querier: Reduce the number of series that are kept in memory while streaming from ingesters. #4745
* [BUGFIX] AlertManager: remove stale template files. #4495
* [BUGFIX] Distributor: fix bug in query-exemplar where some results would get dropped. #4583
* [BUGFIX] Update Thanos dependency: compactor tracing support, azure blocks storage memory fix. #4585
* [BUGFIX] Set appropriate `Content-Type` header for /services endpoint, which previously hard-coded `text/plain`. #4596
* [BUGFIX] Querier: Disable query scheduler SRV DNS lookup, which removes noisy log messages about "failed DNS SRV record lookup". #4601
* [BUGFIX] Memberlist: fixed corrupted packets when sending compound messages with more than 255 messages or messages bigger than 64KB. #4601
* [BUGFIX] Query Frontend: If 'LogQueriesLongerThan' is set to < 0, log all queries as described in the docs. #4633
* [BUGFIX] Distributor: update defaultReplicationStrategy to not fail with extend-write when a single instance is unhealthy. #4636
* [BUGFIX] Distributor: Fix race condition on `/series` introduced by #4683. #4716
* [BUGFIX] Ruler: Fixed leaking notifiers after users are removed #4718
* [BUGFIX] Distributor: Fix a memory leak in distributor due to the cluster label. #4739
* [BUGFIX] Memberlist: Avoid clock skew by limiting the timestamp accepted on gossip. #4750
* [BUGFIX] Compactor: skip compaction if there is only 1 block available for shuffle-sharding compactor. #4756
* [BUGFIX] Compactor: Fixes #4770 - an edge case in compactor with shulffle sharding where compaction stops when a tenant stops ingesting samples. #4771
* [BUGFIX] Compactor: fix cortex_compactor_remaining_planned_compactions not set after plan generation for shuffle sharding compactor. #4772

## 1.11.0 2021-11-25

* [CHANGE] Memberlist: Expose default configuration values to the command line options. Note that setting these explicitly to zero will no longer cause the default to be used. If the default is desired, then do set the option. The following are affected: #4276
  - `-memberlist.stream-timeout`
  - `-memberlist.retransmit-factor`
  - `-memberlist.pull-push-interval`
  - `-memberlist.gossip-interval`
  - `-memberlist.gossip-nodes`
  - `-memberlist.gossip-to-dead-nodes-time`
  - `-memberlist.dead-node-reclaim-time`
* [CHANGE] `-querier.max-fetched-chunks-per-query` previously applied to chunks from ingesters and store separately; now the two combined should not exceed the limit. #4260
* [CHANGE] Memberlist: the metric `memberlist_kv_store_value_bytes` has been removed due to values no longer being stored in-memory as encoded bytes. #4345
* [CHANGE] Some files and directories created by Cortex components on local disk now have stricter permissions, and are only readable by owner, but not group or others. #4394
* [CHANGE] The metric `cortex_deprecated_flags_inuse_total` has been renamed to `deprecated_flags_inuse_total` as part of using grafana/dskit functionality. #4443
* [FEATURE] Ruler: Add new `-ruler.query-stats-enabled` which when enabled will report the `cortex_ruler_query_seconds_total` as a per-user metric that tracks the sum of the wall time of executing queries in the ruler in seconds. #4317
* [FEATURE] Query Frontend: Add `cortex_query_fetched_series_total` and `cortex_query_fetched_chunks_bytes_total` per-user counters to expose the number of series and bytes fetched as part of queries. These metrics can be enabled with the `-frontend.query-stats-enabled` flag (or its respective YAML config option `query_stats_enabled`). #4343
* [FEATURE] AlertManager: Add support for SNS Receiver. #4382
* [FEATURE] Distributor: Add label `status` to metric `cortex_distributor_ingester_append_failures_total` #4442
* [FEATURE] Queries: Added `present_over_time` PromQL function, also some TSDB optimisations. #4505
* [ENHANCEMENT] Add timeout for waiting on compactor to become ACTIVE in the ring. #4262
* [ENHANCEMENT] Reduce memory used by streaming queries, particularly in ruler. #4341
* [ENHANCEMENT] Ring: allow experimental configuration of disabling of heartbeat timeouts by setting the relevant configuration value to zero. Applies to the following: #4342
  * `-distributor.ring.heartbeat-timeout`
  * `-ring.heartbeat-timeout`
  * `-ruler.ring.heartbeat-timeout`
  * `-alertmanager.sharding-ring.heartbeat-timeout`
  * `-compactor.ring.heartbeat-timeout`
  * `-store-gateway.sharding-ring.heartbeat-timeout`
* [ENHANCEMENT] Ring: allow heartbeats to be explicitly disabled by setting the interval to zero. This is considered experimental. This applies to the following configuration options: #4344
  * `-distributor.ring.heartbeat-period`
  * `-ingester.heartbeat-period`
  * `-ruler.ring.heartbeat-period`
  * `-alertmanager.sharding-ring.heartbeat-period`
  * `-compactor.ring.heartbeat-period`
  * `-store-gateway.sharding-ring.heartbeat-period`
* [ENHANCEMENT] Memberlist: optimized receive path for processing ring state updates, to help reduce CPU utilization in large clusters. #4345
* [ENHANCEMENT] Memberlist: expose configuration of memberlist packet compression via `-memberlist.compression=enabled`. #4346
* [ENHANCEMENT] Update Go version to 1.16.6. #4362
* [ENHANCEMENT] Updated Prometheus to include changes from prometheus/prometheus#9083. Now whenever `/labels` API calls include matchers, blocks store is queried for `LabelNames` with matchers instead of `Series` calls which was inefficient. #4380
* [ENHANCEMENT] Querier: performance improvements in socket and memory handling. #4429 #4377
* [ENHANCEMENT] Exemplars are now emitted for all gRPC calls and many operations tracked by histograms. #4462
* [ENHANCEMENT] New options `-server.http-listen-network` and `-server.grpc-listen-network` allow binding as 'tcp4' or 'tcp6'. #4462
* [ENHANCEMENT] Rulers: Using shuffle sharding subring on GetRules API. #4466
* [ENHANCEMENT] Support memcached auto-discovery via `auto-discovery` flag, introduced by thanos in https://github.com/thanos-io/thanos/pull/4487. Both AWS and Google Cloud memcached service support auto-discovery, which returns a list of nodes of the memcached cluster. #4412
* [BUGFIX] Fixes a panic in the query-tee when comparing result. #4465
* [BUGFIX] Frontend: Fixes @ modifier functions (start/end) when splitting queries by time. #4464
* [BUGFIX] Compactor: compactor will no longer try to compact blocks that are already marked for deletion. Previously compactor would consider blocks marked for deletion within `-compactor.deletion-delay / 2` period as eligible for compaction. #4328
* [BUGFIX] HA Tracker: when cleaning up obsolete elected replicas from KV store, tracker didn't update number of cluster per user correctly. #4336
* [BUGFIX] Ruler: fixed counting of PromQL evaluation errors as user-errors when updating `cortex_ruler_queries_failed_total`. #4335
* [BUGFIX] Ingester: When using block storage, prevent any reads or writes while the ingester is stopping. This will prevent accessing TSDB blocks once they have been already closed. #4304
* [BUGFIX] Ingester: fixed ingester stuck on start up (LEAVING ring state) when `-ingester.heartbeat-period=0` and `-ingester.unregister-on-shutdown=false`. #4366
* [BUGFIX] Ingester: panic during shutdown while fetching batches from cache. #4397
* [BUGFIX] Querier: After query-frontend restart, querier may have lower than configured concurrency. #4417
* [BUGFIX] Memberlist: forward only changes, not entire original message. #4419
* [BUGFIX] Memberlist: don't accept old tombstones as incoming change, and don't forward such messages to other gossip members. #4420
* [BUGFIX] Querier: fixed panic when querying exemplars and using `-distributor.shard-by-all-labels=false`. #4473
* [BUGFIX] Querier: honor querier minT,maxT if `nil` SelectHints are passed to Select(). #4413
* [BUGFIX] Compactor: fixed panic while collecting Prometheus metrics. #4483
* [BUGFIX] Update go-kit package to fix spurious log messages #4544

## 1.10.0 / 2021-08-03

* [CHANGE] Prevent path traversal attack from users able to control the HTTP header `X-Scope-OrgID`. #4375 (CVE-2021-36157)
  * Users only have control of the HTTP header when Cortex is not frontend by an auth proxy validating the tenant IDs
* [CHANGE] Enable strict JSON unmarshal for `pkg/util/validation.Limits` struct. The custom `UnmarshalJSON()` will now fail if the input has unknown fields. #4298
* [CHANGE] Cortex chunks storage has been deprecated and it's now in maintenance mode: all Cortex users are encouraged to migrate to the blocks storage. No new features will be added to the chunks storage. The default Cortex configuration still runs the chunks engine; please check out the [blocks storage doc](https://cortexmetrics.io/docs/blocks-storage/) on how to configure Cortex to run with the blocks storage.  #4268
* [CHANGE] The example Kubernetes manifests (stored at `k8s/`) have been removed due to a lack of proper support and maintenance. #4268
* [CHANGE] Querier / ruler: deprecated `-store.query-chunk-limit` CLI flag (and its respective YAML config option `max_chunks_per_query`) in favour of `-querier.max-fetched-chunks-per-query` (and its respective YAML config option `max_fetched_chunks_per_query`). The new limit specifies the maximum number of chunks that can be fetched in a single query from ingesters and long-term storage: the total number of actual fetched chunks could be 2x the limit, being independently applied when querying ingesters and long-term storage. #4125
* [CHANGE] Alertmanager: allowed to configure the experimental receivers firewall on a per-tenant basis. The following CLI flags (and their respective YAML config options) have been changed and moved to the limits config section: #4143
  - `-alertmanager.receivers-firewall.block.cidr-networks` renamed to `-alertmanager.receivers-firewall-block-cidr-networks`
  - `-alertmanager.receivers-firewall.block.private-addresses` renamed to `-alertmanager.receivers-firewall-block-private-addresses`
* [CHANGE] Change default value of `-server.grpc.keepalive.min-time-between-pings` from `5m` to `10s` and `-server.grpc.keepalive.ping-without-stream-allowed` to `true`. #4168
* [CHANGE] Ingester: Change default value of `-ingester.active-series-metrics-enabled` to `true`. This incurs a small increase in memory usage, between 1.2% and 1.6% as measured on ingesters with 1.3M active series. #4257
* [CHANGE] Dependency: update go-redis from v8.2.3 to v8.9.0. #4236
* [FEATURE] Querier: Added new `-querier.max-fetched-series-per-query` flag. When Cortex is running with blocks storage, the max series per query limit is enforced in the querier and applies to unique series received from ingesters and store-gateway (long-term storage). #4179
* [FEATURE] Querier/Ruler: Added new `-querier.max-fetched-chunk-bytes-per-query` flag. When Cortex is running with blocks storage, the max chunk bytes limit is enforced in the querier and ruler and limits the size of all aggregated chunks returned from ingesters and storage as bytes for a query. #4216
* [FEATURE] Alertmanager: support negative matchers, time-based muting - [upstream release notes](https://github.com/prometheus/alertmanager/releases/tag/v0.22.0). #4237
* [FEATURE] Alertmanager: Added rate-limits to notifiers. Rate limits used by all integrations can be configured using `-alertmanager.notification-rate-limit`, while per-integration rate limits can be specified via `-alertmanager.notification-rate-limit-per-integration` parameter. Both shared and per-integration limits can be overwritten using overrides mechanism. These limits are applied on individual (per-tenant) alertmanagers. Rate-limited notifications are failed notifications. It is possible to monitor rate-limited notifications via new `cortex_alertmanager_notification_rate_limited_total` metric. #4135 #4163
* [FEATURE] Alertmanager: Added `-alertmanager.max-config-size-bytes` limit to control size of configuration files that Cortex users can upload to Alertmanager via API. This limit is configurable per-tenant. #4201
* [FEATURE] Alertmanager: Added `-alertmanager.max-templates-count` and `-alertmanager.max-template-size-bytes` options to control number and size of templates uploaded to Alertmanager via API. These limits are configurable per-tenant. #4223
* [FEATURE] Added flag `-debug.block-profile-rate` to enable goroutine blocking events profiling. #4217
* [FEATURE] Alertmanager: The experimental sharding feature is now considered complete. Detailed information about the configuration options can be found [here for alertmanager](https://cortexmetrics.io/docs/configuration/configuration-file/#alertmanager_config) and [here for the alertmanager storage](https://cortexmetrics.io/docs/configuration/configuration-file/#alertmanager_storage_config). To use the feature: #3925 #4020 #4021 #4031 #4084 #4110 #4126 #4127 #4141 #4146 #4161 #4162 #4222
  * Ensure that a remote storage backend is configured for Alertmanager to store state using `-alertmanager-storage.backend`, and flags related to the backend. Note that the `local` and `configdb` storage backends are not supported.
  * Ensure that a ring store is configured using `-alertmanager.sharding-ring.store`, and set the flags relevant to the chosen store type.
  * Enable the feature using `-alertmanager.sharding-enabled`.
  * Note the prior addition of a new configuration option `-alertmanager.persist-interval`. This sets the interval between persisting the current alertmanager state (notification log and silences) to object storage. See the [configuration file reference](https://cortexmetrics.io/docs/configuration/configuration-file/#alertmanager_config) for more information.
* [ENHANCEMENT] Alertmanager: Cleanup persisted state objects from remote storage when a tenant configuration is deleted. #4167
* [ENHANCEMENT] Storage: Added the ability to disable Open Census within GCS client (e.g `-gcs.enable-opencensus=false`). #4219
* [ENHANCEMENT] Etcd: Added username and password to etcd config. #4205
* [ENHANCEMENT] Alertmanager: introduced new metrics to monitor operation when using `-alertmanager.sharding-enabled`: #4149
  * `cortex_alertmanager_state_fetch_replica_state_total`
  * `cortex_alertmanager_state_fetch_replica_state_failed_total`
  * `cortex_alertmanager_state_initial_sync_total`
  * `cortex_alertmanager_state_initial_sync_completed_total`
  * `cortex_alertmanager_state_initial_sync_duration_seconds`
  * `cortex_alertmanager_state_persist_total`
  * `cortex_alertmanager_state_persist_failed_total`
* [ENHANCEMENT] Blocks storage: support ingesting exemplars and querying of exemplars.  Enabled by setting new CLI flag `-blocks-storage.tsdb.max-exemplars=<n>` or config option `blocks_storage.tsdb.max_exemplars` to positive value. #4124 #4181
* [ENHANCEMENT] Distributor: Added distributors ring status section in the admin page. #4151
* [ENHANCEMENT] Added zone-awareness support to alertmanager for use when sharding is enabled. When zone-awareness is enabled, alerts will be replicated across availability zones. #4204
* [ENHANCEMENT] Added `tenant_ids` tag to tracing spans #4186
* [ENHANCEMENT] Ring, query-frontend: Avoid using automatic private IPs (APIPA) when discovering IP address from the interface during the registration of the instance in the ring, or by query-frontend when used with query-scheduler. APIPA still used as last resort with logging indicating usage. #4032
* [ENHANCEMENT] Memberlist: introduced new metrics to aid troubleshooting tombstone convergence: #4231
  * `memberlist_client_kv_store_value_tombstones`
  * `memberlist_client_kv_store_value_tombstones_removed_total`
  * `memberlist_client_messages_to_broadcast_dropped_total`
* [ENHANCEMENT] Alertmanager: Added `-alertmanager.max-dispatcher-aggregation-groups` option to control max number of active dispatcher groups in Alertmanager (per tenant, also overridable). When the limit is reached, Dispatcher produces log message and increases `cortex_alertmanager_dispatcher_aggregation_group_limit_reached_total` metric. #4254
* [ENHANCEMENT] Alertmanager: Added `-alertmanager.max-alerts-count` and `-alertmanager.max-alerts-size-bytes` to control max number of alerts and total size of alerts that a single user can have in Alertmanager's memory. Adding more alerts will fail with a log message and incrementing `cortex_alertmanager_alerts_insert_limited_total` metric (per-user). These limits can be overrided by using per-tenant overrides. Current values are tracked in `cortex_alertmanager_alerts_limiter_current_alerts` and `cortex_alertmanager_alerts_limiter_current_alerts_size_bytes` metrics. #4253
* [ENHANCEMENT] Store-gateway: added `-store-gateway.sharding-ring.wait-stability-min-duration` and `-store-gateway.sharding-ring.wait-stability-max-duration` support to store-gateway, to wait for ring stability at startup. #4271
* [ENHANCEMENT] Ruler: added `rule_group` label to metrics `cortex_prometheus_rule_group_iterations_total` and `cortex_prometheus_rule_group_iterations_missed_total`. #4121
* [ENHANCEMENT] Ruler: added new metrics for tracking total number of queries and push requests sent to ingester, as well as failed queries and push requests. Failures are only counted for internal errors, but not user-errors like limits or invalid query. This is in contrast to existing `cortex_prometheus_rule_evaluation_failures_total`, which is incremented also when query or samples appending fails due to user-errors. #4281
  * `cortex_ruler_write_requests_total`
  * `cortex_ruler_write_requests_failed_total`
  * `cortex_ruler_queries_total`
  * `cortex_ruler_queries_failed_total`
* [ENHANCEMENT] Ingester: Added option `-ingester.ignore-series-limit-for-metric-names` with comma-separated list of metric names that will be ignored in max series per metric limit. #4302
* [ENHANCEMENT] Added instrumentation to Redis client, with the following metrics: #3976
  - `cortex_rediscache_request_duration_seconds`
* [BUGFIX] Purger: fix `Invalid null value in condition for column range` caused by `nil` value in range for WriteBatch query. #4128
* [BUGFIX] Ingester: fixed infrequent panic caused by a race condition between TSDB mmap-ed head chunks truncation and queries. #4176
* [BUGFIX] Alertmanager: fix Alertmanager status page if clustering via gossip is disabled or sharding is enabled. #4184
* [BUGFIX] Ruler: fix `/ruler/rule_groups` endpoint doesn't work when used with object store. #4182
* [BUGFIX] Ruler: Honor the evaluation delay for the `ALERTS` and `ALERTS_FOR_STATE` series. #4227
* [BUGFIX] Make multiple Get requests instead of MGet on Redis Cluster. #4056
* [BUGFIX] Ingester: fix issue where runtime limits erroneously override default limits. #4246
* [BUGFIX] Ruler: fix startup in single-binary mode when the new `ruler_storage` is used. #4252
* [BUGFIX] Querier: fix queries failing with "at least 1 healthy replica required, could only find 0" error right after scaling up store-gateways until they're ACTIVE in the ring. #4263
* [BUGFIX] Store-gateway: when blocks sharding is enabled, do not load all blocks in each store-gateway in case of a cold startup, but load only blocks owned by the store-gateway replica. #4271
* [BUGFIX] Memberlist: fix to setting the default configuration value for `-memberlist.retransmit-factor` when not provided. This should improve propagation delay of the ring state (including, but not limited to, tombstones). Note that if the configuration is already explicitly given, this fix has no effect. #4269
* [BUGFIX] Querier: Fix issue where samples in a chunk might get skipped by batch iterator. #4218

## Blocksconvert

* [ENHANCEMENT] Scanner: add support for DynamoDB (v9 schema only). #3828
* [ENHANCEMENT] Add Cassandra support. #3795
* [ENHANCEMENT] Scanner: retry failed uploads. #4188

## 1.9.0 / 2021-05-14

* [CHANGE] Alertmanager now removes local files after Alertmanager is no longer running for removed or resharded user. #3910
* [CHANGE] Alertmanager now stores local files in per-tenant folders. Files stored by Alertmanager previously are migrated to new hierarchy. Support for this migration will be removed in Cortex 1.11. #3910
* [CHANGE] Ruler: deprecated `-ruler.storage.*` CLI flags (and their respective YAML config options) in favour of `-ruler-storage.*`. The deprecated config will be removed in Cortex 1.11. #3945
* [CHANGE] Alertmanager: deprecated `-alertmanager.storage.*` CLI flags (and their respective YAML config options) in favour of `-alertmanager-storage.*`. This change doesn't apply to `alertmanager.storage.path` and `alertmanager.storage.retention`. The deprecated config will be removed in Cortex 1.11. #4002
* [CHANGE] Alertmanager: removed `-cluster.` CLI flags deprecated in Cortex 1.7. The new config options to use are: #3946
  * `-alertmanager.cluster.listen-address` instead of `-cluster.listen-address`
  * `-alertmanager.cluster.advertise-address` instead of `-cluster.advertise-address`
  * `-alertmanager.cluster.peers` instead of `-cluster.peer`
  * `-alertmanager.cluster.peer-timeout` instead of `-cluster.peer-timeout`
* [CHANGE] Blocks storage: removed the config option `-blocks-storage.bucket-store.index-cache.postings-compression-enabled`, which was deprecated in Cortex 1.6. Postings compression is always enabled. #4101
* [CHANGE] Querier: removed the config option `-store.max-look-back-period`, which was deprecated in Cortex 1.6 and was used only by the chunks storage. You should use `-querier.max-query-lookback` instead. #4101
* [CHANGE] Query Frontend: removed the config option `-querier.compress-http-responses`, which was deprecated in Cortex 1.6. You should use`-api.response-compression-enabled` instead. #4101
* [CHANGE] Runtime-config / overrides: removed the config options `-limits.per-user-override-config` (use `-runtime-config.file`) and `-limits.per-user-override-period` (use `-runtime-config.reload-period`), both deprecated since Cortex 0.6.0. #4112
* [CHANGE] Cortex now fails fast on startup if unable to connect to the ring backend. #4068
* [FEATURE] The following features have been marked as stable: #4101
  - Shuffle-sharding
  - Querier support for querying chunks and blocks store at the same time
  - Tracking of active series and exporting them as metrics (`-ingester.active-series-metrics-enabled` and related flags)
  - Blocks storage: lazy mmap of block indexes in the store-gateway (`-blocks-storage.bucket-store.index-header-lazy-loading-enabled`)
  - Ingester: close idle TSDB and remove them from local disk (`-blocks-storage.tsdb.close-idle-tsdb-timeout`)
* [FEATURE] Memberlist: add TLS configuration options for the memberlist transport layer used by the gossip KV store. #4046
  * New flags added for memberlist communication:
    * `-memberlist.tls-enabled`
    * `-memberlist.tls-cert-path`
    * `-memberlist.tls-key-path`
    * `-memberlist.tls-ca-path`
    * `-memberlist.tls-server-name`
    * `-memberlist.tls-insecure-skip-verify`
* [FEATURE] Ruler: added `local` backend support to the ruler storage configuration under the `-ruler-storage.` flag prefix. #3932
* [ENHANCEMENT] Upgraded Docker base images to `alpine:3.13`. #4042
* [ENHANCEMENT] Blocks storage: reduce ingester memory by eliminating series reference cache. #3951
* [ENHANCEMENT] Ruler: optimized `<prefix>/api/v1/rules` and `<prefix>/api/v1/alerts` when ruler sharding is enabled. #3916
* [ENHANCEMENT] Ruler: added the following metrics when ruler sharding is enabled: #3916
  * `cortex_ruler_clients`
  * `cortex_ruler_client_request_duration_seconds`
* [ENHANCEMENT] Alertmanager: Add API endpoint to list all tenant alertmanager configs: `GET /multitenant_alertmanager/configs`. #3529
* [ENHANCEMENT] Ruler: Add API endpoint to list all tenant ruler rule groups: `GET /ruler/rule_groups`. #3529
* [ENHANCEMENT] Query-frontend/scheduler: added querier forget delay (`-query-frontend.querier-forget-delay` and `-query-scheduler.querier-forget-delay`) to mitigate the blast radius in the event queriers crash because of a repeatedly sent "query of death" when shuffle-sharding is enabled. #3901
* [ENHANCEMENT] Query-frontend: reduced memory allocations when serializing query response. #3964
* [ENHANCEMENT] Querier / ruler: some optimizations to PromQL query engine. #3934 #3989
* [ENHANCEMENT] Ingester: reduce CPU and memory when an high number of errors are returned by the ingester on the write path with the blocks storage. #3969 #3971 #3973
* [ENHANCEMENT] Distributor: reduce CPU and memory when an high number of errors are returned by the distributor on the write path. #3990
* [ENHANCEMENT] Put metric before label value in the "label value too long" error message. #4018
* [ENHANCEMENT] Allow use of `y|w|d` suffixes for duration related limits and per-tenant limits. #4044
* [ENHANCEMENT] Query-frontend: Small optimization on top of PR #3968 to avoid unnecessary Extents merging. #4026
* [ENHANCEMENT] Add a metric `cortex_compactor_compaction_interval_seconds` for the compaction interval config value. #4040
* [ENHANCEMENT] Ingester: added following per-ingester (instance) experimental limits: max number of series in memory (`-ingester.instance-limits.max-series`), max number of users in memory (`-ingester.instance-limits.max-tenants`), max ingestion rate (`-ingester.instance-limits.max-ingestion-rate`), and max inflight requests (`-ingester.instance-limits.max-inflight-push-requests`). These limits are only used when using blocks storage. Limits can also be configured using runtime-config feature, and current values are exported as `cortex_ingester_instance_limits` metric. #3992.
* [ENHANCEMENT] Cortex is now built with Go 1.16. #4062
* [ENHANCEMENT] Distributor: added per-distributor experimental limits: max number of inflight requests (`-distributor.instance-limits.max-inflight-push-requests`) and max ingestion rate in samples/sec (`-distributor.instance-limits.max-ingestion-rate`). If not set, these two are unlimited. Also added metrics to expose current values (`cortex_distributor_inflight_push_requests`, `cortex_distributor_ingestion_rate_samples_per_second`) as well as limits (`cortex_distributor_instance_limits` with various `limit` label values). #4071
* [ENHANCEMENT] Ruler: Added `-ruler.enabled-tenants` and `-ruler.disabled-tenants` to explicitly enable or disable rules processing for specific tenants. #4074
* [ENHANCEMENT] Block Storage Ingester: `/flush` now accepts two new parameters: `tenant` to specify tenant to flush and `wait=true` to make call synchronous. Multiple tenants can be specified by repeating `tenant` parameter. If no `tenant` is specified, all tenants are flushed, as before. #4073
* [ENHANCEMENT] Alertmanager: validate configured `-alertmanager.web.external-url` and fail if ends with `/`. #4081
* [ENHANCEMENT] Alertmanager: added `-alertmanager.receivers-firewall.block.cidr-networks` and `-alertmanager.receivers-firewall.block.private-addresses` to block specific network addresses in HTTP-based Alertmanager receiver integrations. #4085
* [ENHANCEMENT] Allow configuration of Cassandra's host selection policy. #4069
* [ENHANCEMENT] Store-gateway: retry syncing blocks if a per-tenant sync fails. #3975 #4088
* [ENHANCEMENT] Add metric `cortex_tcp_connections` exposing the current number of accepted TCP connections. #4099
* [ENHANCEMENT] Querier: Allow federated queries to run concurrently. #4065
* [ENHANCEMENT] Label Values API call now supports `match[]` parameter when querying blocks on storage (assuming `-querier.query-store-for-labels-enabled` is enabled). #4133
* [BUGFIX] Ruler-API: fix bug where `/api/v1/rules/<namespace>/<group_name>` endpoint return `400` instead of `404`. #4013
* [BUGFIX] Distributor: reverted changes done to rate limiting in #3825. #3948
* [BUGFIX] Ingester: Fix race condition when opening and closing tsdb concurrently. #3959
* [BUGFIX] Querier: streamline tracing spans. #3924
* [BUGFIX] Ruler Storage: ignore objects with empty namespace or group in the name. #3999
* [BUGFIX] Distributor: fix issue causing distributors to not extend the replication set because of failing instances when zone-aware replication is enabled. #3977
* [BUGFIX] Query-frontend: Fix issue where cached entry size keeps increasing when making tiny query repeatedly. #3968
* [BUGFIX] Compactor: `-compactor.blocks-retention-period` now supports weeks (`w`) and years (`y`). #4027
* [BUGFIX] Querier: returning 422 (instead of 500) when query hits `max_chunks_per_query` limit with block storage, when the limit is hit in the store-gateway. #3937
* [BUGFIX] Ruler: Rule group limit enforcement should now allow the same number of rules in a group as the limit. #3616
* [BUGFIX] Frontend, Query-scheduler: allow querier to notify about shutdown without providing any authentication. #4066
* [BUGFIX] Querier: fixed race condition causing queries to fail right after querier startup with the "empty ring" error. #4068
* [BUGFIX] Compactor: Increment `cortex_compactor_runs_failed_total` if compactor failed compact a single tenant. #4094
* [BUGFIX] Tracing: hot fix to avoid the Jaeger tracing client to indefinitely block the Cortex process shutdown in case the HTTP connection to the tracing backend is blocked. #4134
* [BUGFIX] Forward proper EndsAt from ruler to Alertmanager inline with Prometheus behaviour. #4017
* [BUGFIX] Querier: support filtering LabelValues with matchers when using tenant federation. #4277

## Blocksconvert

* [ENHANCEMENT] Builder: add `-builder.timestamp-tolerance` option which may reduce block size by rounding timestamps to make difference whole seconds. #3891

## 1.8.1 / 2021-04-27

* [CHANGE] Fix for CVE-2021-31232: Local file disclosure vulnerability when `-experimental.alertmanager.enable-api` is used. The HTTP basic auth `password_file` can be used as an attack vector to send any file content via a webhook. The alertmanager templates can be used as an attack vector to send any file content because the alertmanager can load any text file specified in the templates list.

## 1.8.0 / 2021-03-24

* [CHANGE] Alertmanager: Don't expose cluster information to tenants via the `/alertmanager/api/v1/status` API endpoint when operating with clustering enabled. #3903
* [CHANGE] Ingester: don't update internal "last updated" timestamp of TSDB if tenant only sends invalid samples. This affects how "idle" time is computed. #3727
* [CHANGE] Require explicit flag `-<prefix>.tls-enabled` to enable TLS in GRPC clients. Previously it was enough to specify a TLS flag to enable TLS validation. #3156
* [CHANGE] Query-frontend: removed `-querier.split-queries-by-day` (deprecated in Cortex 0.4.0). Please use `-querier.split-queries-by-interval` instead. #3813
* [CHANGE] Store-gateway: the chunks pool controlled by `-blocks-storage.bucket-store.max-chunk-pool-bytes` is now shared across all tenants. #3830
* [CHANGE] Ingester: return error code 400 instead of 429 when per-user/per-tenant series/metadata limits are reached. #3833
* [CHANGE] Compactor: add `reason` label to `cortex_compactor_blocks_marked_for_deletion_total` metric. Source blocks marked for deletion by compactor are labelled as `compaction`, while blocks passing the retention period are labelled as `retention`. #3879
* [CHANGE] Alertmanager: the `DELETE /api/v1/alerts` is now idempotent. No error is returned if the alertmanager config doesn't exist. #3888
* [FEATURE] Experimental Ruler Storage: Add a separate set of configuration options to configure the ruler storage backend under the `-ruler-storage.` flag prefix. All blocks storage bucket clients and the config service are currently supported. Clients using this implementation will only be enabled if the existing `-ruler.storage` flags are left unset. #3805 #3864
* [FEATURE] Experimental Alertmanager Storage: Add a separate set of configuration options to configure the alertmanager storage backend under the `-alertmanager-storage.` flag prefix. All blocks storage bucket clients and the config service are currently supported. Clients using this implementation will only be enabled if the existing `-alertmanager.storage` flags are left unset. #3888
* [FEATURE] Adds support to S3 server-side encryption using KMS. The S3 server-side encryption config can be overridden on a per-tenant basis for the blocks storage, ruler and alertmanager. Deprecated `-<prefix>.s3.sse-encryption`, please use the following CLI flags that have been added. #3651 #3810 #3811 #3870 #3886 #3906
  - `-<prefix>.s3.sse.type`
  - `-<prefix>.s3.sse.kms-key-id`
  - `-<prefix>.s3.sse.kms-encryption-context`
* [FEATURE] Querier: Enable `@ <timestamp>` modifier in PromQL using the new `-querier.at-modifier-enabled` flag. #3744
* [FEATURE] Overrides Exporter: Add `overrides-exporter` module for exposing per-tenant resource limit overrides as metrics. It is not included in `all` target (single-binary mode), and must be explicitly enabled. #3785
* [FEATURE] Experimental thanosconvert: introduce an experimental tool `thanosconvert` to migrate Thanos block metadata to Cortex metadata. #3770
* [FEATURE] Alertmanager: It now shards the `/api/v1/alerts` API using the ring when sharding is enabled. #3671
  * Added `-alertmanager.max-recv-msg-size` (defaults to 16M) to limit the size of HTTP request body handled by the alertmanager.
  * New flags added for communication between alertmanagers:
    * `-alertmanager.max-recv-msg-size`
    * `-alertmanager.alertmanager-client.remote-timeout`
    * `-alertmanager.alertmanager-client.tls-enabled`
    * `-alertmanager.alertmanager-client.tls-cert-path`
    * `-alertmanager.alertmanager-client.tls-key-path`
    * `-alertmanager.alertmanager-client.tls-ca-path`
    * `-alertmanager.alertmanager-client.tls-server-name`
    * `-alertmanager.alertmanager-client.tls-insecure-skip-verify`
* [FEATURE] Compactor: added blocks storage per-tenant retention support. This is configured via `-compactor.retention-period`, and can be overridden on a per-tenant basis. #3879
* [ENHANCEMENT] Queries: Instrument queries that were discarded due to the configured `max_outstanding_requests_per_tenant`. #3894
  * `cortex_query_frontend_discarded_requests_total`
  * `cortex_query_scheduler_discarded_requests_total`
* [ENHANCEMENT] Ruler: Add TLS and explicit basis authentication configuration options for the HTTP client the ruler uses to communicate with the alertmanager. #3752
  * `-ruler.alertmanager-client.basic-auth-username`: Configure the basic authentication username used by the client. Takes precedent over a URL configured username.
  * `-ruler.alertmanager-client.basic-auth-password`: Configure the basic authentication password used by the client. Takes precedent over a URL configured password.
  * `-ruler.alertmanager-client.tls-ca-path`: File path to the CA file.
  * `-ruler.alertmanager-client.tls-cert-path`: File path to the TLS certificate.
  * `-ruler.alertmanager-client.tls-insecure-skip-verify`: Boolean to disable verifying the certificate.
  * `-ruler.alertmanager-client.tls-key-path`: File path to the TLS key certificate.
  * `-ruler.alertmanager-client.tls-server-name`: Expected name on the TLS certificate.
* [ENHANCEMENT] Ingester: exposed metric `cortex_ingester_oldest_unshipped_block_timestamp_seconds`, tracking the unix timestamp of the oldest TSDB block not shipped to the storage yet. #3705
* [ENHANCEMENT] Prometheus upgraded. #3739 #3806
  * Avoid unnecessary `runtime.GC()` during compactions.
  * Prevent compaction loop in TSDB on data gap.
* [ENHANCEMENT] Query-Frontend now returns server side performance metrics using `Server-Timing` header when query stats is enabled. #3685
* [ENHANCEMENT] Runtime Config: Add a `mode` query parameter for the runtime config endpoint. `/runtime_config?mode=diff` now shows the YAML runtime configuration with all values that differ from the defaults. #3700
* [ENHANCEMENT] Distributor: Enable downstream projects to wrap distributor push function and access the deserialized write requests before/after they are pushed. #3755
* [ENHANCEMENT] Add flag `-<prefix>.tls-server-name` to require a specific server name instead of the hostname on the certificate. #3156
* [ENHANCEMENT] Alertmanager: Remove a tenant's alertmanager instead of pausing it as we determine it is no longer needed. #3722
* [ENHANCEMENT] Blocks storage: added more configuration options to S3 client. #3775
  * `-blocks-storage.s3.tls-handshake-timeout`: Maximum time to wait for a TLS handshake. 0 means no limit.
  * `-blocks-storage.s3.expect-continue-timeout`: The time to wait for a server's first response headers after fully writing the request headers if the request has an Expect header. 0 to send the request body immediately.
  * `-blocks-storage.s3.max-idle-connections`: Maximum number of idle (keep-alive) connections across all hosts. 0 means no limit.
  * `-blocks-storage.s3.max-idle-connections-per-host`: Maximum number of idle (keep-alive) connections to keep per-host. If 0, a built-in default value is used.
  * `-blocks-storage.s3.max-connections-per-host`: Maximum number of connections per host. 0 means no limit.
* [ENHANCEMENT] Ingester: when tenant's TSDB is closed, Ingester now removes pushed metrics-metadata from memory, and removes metadata (`cortex_ingester_memory_metadata`, `cortex_ingester_memory_metadata_created_total`, `cortex_ingester_memory_metadata_removed_total`) and validation metrics (`cortex_discarded_samples_total`, `cortex_discarded_metadata_total`). #3782
* [ENHANCEMENT] Distributor: cleanup metrics for inactive tenants. #3784
* [ENHANCEMENT] Ingester: Have ingester to re-emit following TSDB metrics. #3800
  * `cortex_ingester_tsdb_blocks_loaded`
  * `cortex_ingester_tsdb_reloads_total`
  * `cortex_ingester_tsdb_reloads_failures_total`
  * `cortex_ingester_tsdb_symbol_table_size_bytes`
  * `cortex_ingester_tsdb_storage_blocks_bytes`
  * `cortex_ingester_tsdb_time_retentions_total`
* [ENHANCEMENT] Querier: distribute workload across `-store-gateway.sharding-ring.replication-factor` store-gateway replicas when querying blocks and `-store-gateway.sharding-enabled=true`. #3824
* [ENHANCEMENT] Distributor / HA Tracker: added cleanup of unused elected HA replicas from KV store. Added following metrics to monitor this process: #3809
  * `cortex_ha_tracker_replicas_cleanup_started_total`
  * `cortex_ha_tracker_replicas_cleanup_marked_for_deletion_total`
  * `cortex_ha_tracker_replicas_cleanup_deleted_total`
  * `cortex_ha_tracker_replicas_cleanup_delete_failed_total`
* [ENHANCEMENT] Ruler now has new API endpoint `/ruler/delete_tenant_config` that can be used to delete all ruler groups for tenant. It is intended to be used by administrators who wish to clean up state after removed user. Note that this endpoint is enabled regardless of `-experimental.ruler.enable-api`. #3750 #3899
* [ENHANCEMENT] Query-frontend, query-scheduler: cleanup metrics for inactive tenants. #3826
* [ENHANCEMENT] Blocks storage: added `-blocks-storage.s3.region` support to S3 client configuration. #3811
* [ENHANCEMENT] Distributor: Remove cached subrings for inactive users when using shuffle sharding. #3849
* [ENHANCEMENT] Store-gateway: Reduced memory used to fetch chunks at query time. #3855
* [ENHANCEMENT] Ingester: attempt to prevent idle compaction from happening in concurrent ingesters by introducing a 25% jitter to the configured idle timeout (`-blocks-storage.tsdb.head-compaction-idle-timeout`). #3850
* [ENHANCEMENT] Compactor: cleanup local files for users that are no longer owned by compactor. #3851
* [ENHANCEMENT] Store-gateway: close empty bucket stores, and delete leftover local files for tenants that no longer belong to store-gateway. #3853
* [ENHANCEMENT] Store-gateway: added metrics to track partitioner behaviour. #3877
  * `cortex_bucket_store_partitioner_requested_bytes_total`
  * `cortex_bucket_store_partitioner_requested_ranges_total`
  * `cortex_bucket_store_partitioner_expanded_bytes_total`
  * `cortex_bucket_store_partitioner_expanded_ranges_total`
* [ENHANCEMENT] Store-gateway: added metrics to monitor chunk buffer pool behaviour. #3880
  * `cortex_bucket_store_chunk_pool_requested_bytes_total`
  * `cortex_bucket_store_chunk_pool_returned_bytes_total`
* [ENHANCEMENT] Alertmanager: load alertmanager configurations from object storage concurrently, and only load necessary configurations, speeding configuration synchronization process and executing fewer "GET object" operations to the storage when sharding is enabled. #3898
* [ENHANCEMENT] Ingester (blocks storage): Ingester can now stream entire chunks instead of individual samples to the querier. At the moment this feature must be explicitly enabled either by using `-ingester.stream-chunks-when-using-blocks` flag or `ingester_stream_chunks_when_using_blocks` (boolean) field in runtime config file, but these configuration options are temporary and will be removed when feature is stable. #3889
* [ENHANCEMENT] Alertmanager: New endpoint `/multitenant_alertmanager/delete_tenant_config` to delete configuration for tenant identified by `X-Scope-OrgID` header. This is an internal endpoint, available even if Alertmanager API is not enabled by using `-experimental.alertmanager.enable-api`. #3900
* [ENHANCEMENT] MemCached: Add `max_item_size` support. #3929
* [BUGFIX] Cortex: Fixed issue where fatal errors and various log messages where not logged. #3778
* [BUGFIX] HA Tracker: don't track as error in the `cortex_kv_request_duration_seconds` metric a CAS operation intentionally aborted. #3745
* [BUGFIX] Querier / ruler: do not log "error removing stale clients" if the ring is empty. #3761
* [BUGFIX] Store-gateway: fixed a panic caused by a race condition when the index-header lazy loading is enabled. #3775 #3789
* [BUGFIX] Compactor: fixed "could not guess file size" log when uploading blocks deletion marks to the global location. #3807
* [BUGFIX] Prevent panic at start if the http_prefix setting doesn't have a valid value. #3796
* [BUGFIX] Memberlist: fixed panic caused by race condition in `armon/go-metrics` used by memberlist client. #3725
* [BUGFIX] Querier: returning 422 (instead of 500) when query hits `max_chunks_per_query` limit with block storage. #3895
* [BUGFIX] Alertmanager: Ensure that experimental `/api/v1/alerts` endpoints work when `-http.prefix` is empty. #3905
* [BUGFIX] Chunk store: fix panic in inverted index when deleted fingerprint is no longer in the index. #3543

## 1.7.1 / 2021-04-27

* [CHANGE] Fix for CVE-2021-31232: Local file disclosure vulnerability when `-experimental.alertmanager.enable-api` is used. The HTTP basic auth `password_file` can be used as an attack vector to send any file content via a webhook. The alertmanager templates can be used as an attack vector to send any file content because the alertmanager can load any text file specified in the templates list.

## 1.7.0 / 2021-02-23

Note the blocks storage compactor runs a migration task at startup in this version, which can take many minutes and use a lot of RAM.
[Turn this off after first run](https://cortexmetrics.io/docs/blocks-storage/production-tips/#ensure-deletion-marks-migration-is-disabled-after-first-run).

* [CHANGE] FramedSnappy encoding support has been removed from Push and Remote Read APIs. This means Prometheus 1.6 support has been removed and the oldest Prometheus version supported in the remote write is 1.7. #3682
* [CHANGE] Ruler: removed the flag `-ruler.evaluation-delay-duration-deprecated` which was deprecated in 1.4.0. Please use the `ruler_evaluation_delay_duration` per-tenant limit instead. #3694
* [CHANGE] Removed the flags `-<prefix>.grpc-use-gzip-compression` which were deprecated in 1.3.0: #3694
  * `-query-scheduler.grpc-client-config.grpc-use-gzip-compression`: use `-query-scheduler.grpc-client-config.grpc-compression` instead
  * `-frontend.grpc-client-config.grpc-use-gzip-compression`: use `-frontend.grpc-client-config.grpc-compression` instead
  * `-ruler.client.grpc-use-gzip-compression`: use `-ruler.client.grpc-compression` instead
  * `-bigtable.grpc-use-gzip-compression`: use `-bigtable.grpc-compression` instead
  * `-ingester.client.grpc-use-gzip-compression`: use `-ingester.client.grpc-compression` instead
  * `-querier.frontend-client.grpc-use-gzip-compression`: use `-querier.frontend-client.grpc-compression` instead
* [CHANGE] Querier: it's not required to set `-frontend.query-stats-enabled=true` in the querier anymore to enable query statistics logging in the query-frontend. The flag is now required to be configured only in the query-frontend and it will be propagated to the queriers. #3595 #3695
* [CHANGE] Blocks storage: compactor is now required when running a Cortex cluster with the blocks storage, because it also keeps the bucket index updated. #3583
* [CHANGE] Blocks storage: block deletion marks are now stored in a per-tenant global markers/ location too, other than within the block location. The compactor, at startup, will copy deletion marks from the block location to the global location. This migration is required only once, so it can be safely disabled via `-compactor.block-deletion-marks-migration-enabled=false` after new compactor has successfully started at least once in the cluster. #3583
* [CHANGE] OpenStack Swift: the default value for the `-ruler.storage.swift.container-name` and `-swift.container-name` config options has changed from `cortex` to empty string. If you were relying on the default value, please set it back to `cortex`. #3660
* [CHANGE] HA Tracker: configured replica label is now verified against label value length limit (`-validation.max-length-label-value`). #3668
* [CHANGE] Distributor: `extend_writes` field in YAML configuration has moved from `lifecycler` (inside `ingester_config`) to `distributor_config`. This doesn't affect command line option `-distributor.extend-writes`, which stays the same. #3719
* [CHANGE] Alertmanager: Deprecated `-cluster.` CLI flags in favor of their `-alertmanager.cluster.` equivalent. The deprecated flags (and their respective YAML config options) are: #3677
  * `-cluster.listen-address` in favor of `-alertmanager.cluster.listen-address`
  * `-cluster.advertise-address` in favor of `-alertmanager.cluster.advertise-address`
  * `-cluster.peer` in favor of `-alertmanager.cluster.peers`
  * `-cluster.peer-timeout` in favor of `-alertmanager.cluster.peer-timeout`
* [CHANGE] Blocks storage: the default value of `-blocks-storage.bucket-store.sync-interval` has been changed from `5m` to `15m`. #3724
* [FEATURE] Querier: Queries can be federated across multiple tenants. The tenants IDs involved need to be specified separated by a `|` character in the `X-Scope-OrgID` request header. This is an experimental feature, which can be enabled by setting `-tenant-federation.enabled=true` on all Cortex services. #3250
* [FEATURE] Alertmanager: introduced the experimental option `-alertmanager.sharding-enabled` to shard tenants across multiple Alertmanager instances. This feature is still under heavy development and its usage is discouraged. The following new metrics are exported by the Alertmanager: #3664
  * `cortex_alertmanager_ring_check_errors_total`
  * `cortex_alertmanager_sync_configs_total`
  * `cortex_alertmanager_sync_configs_failed_total`
  * `cortex_alertmanager_tenants_discovered`
  * `cortex_alertmanager_tenants_owned`
* [ENHANCEMENT] Allow specifying JAEGER_ENDPOINT instead of sampling server or local agent port. #3682
* [ENHANCEMENT] Blocks storage: introduced a per-tenant bucket index, periodically updated by the compactor, used to avoid full bucket scanning done by queriers, store-gateways and rulers. The bucket index is updated by the compactor during blocks cleanup, on every `-compactor.cleanup-interval`. #3553 #3555 #3561 #3583 #3625 #3711 #3715
* [ENHANCEMENT] Blocks storage: introduced an option `-blocks-storage.bucket-store.bucket-index.enabled` to enable the usage of the bucket index in the querier, store-gateway and ruler. When enabled, the querier, store-gateway and ruler will use the bucket index to find a tenant's blocks instead of running the periodic bucket scan. The following new metrics are exported by the querier and ruler: #3614 #3625
  * `cortex_bucket_index_loads_total`
  * `cortex_bucket_index_load_failures_total`
  * `cortex_bucket_index_load_duration_seconds`
  * `cortex_bucket_index_loaded`
* [ENHANCEMENT] Compactor: exported the following metrics. #3583 #3625
  * `cortex_bucket_blocks_count`: Total number of blocks per tenant in the bucket. Includes blocks marked for deletion, but not partial blocks.
  * `cortex_bucket_blocks_marked_for_deletion_count`: Total number of blocks per tenant marked for deletion in the bucket.
  * `cortex_bucket_blocks_partials_count`: Total number of partial blocks.
  * `cortex_bucket_index_last_successful_update_timestamp_seconds`: Timestamp of the last successful update of a tenant's bucket index.
* [ENHANCEMENT] Ruler: Add `cortex_prometheus_last_evaluation_samples` to expose the number of samples generated by a rule group per tenant. #3582
* [ENHANCEMENT] Memberlist: add status page (/memberlist) with available details about memberlist-based KV store and memberlist cluster. It's also possible to view KV values in Go struct or JSON format, or download for inspection. #3575
* [ENHANCEMENT] Memberlist: client can now keep a size-bounded buffer with sent and received messages and display them in the admin UI (/memberlist) for troubleshooting. #3581 #3602
* [ENHANCEMENT] Blocks storage: added block index attributes caching support to metadata cache. The TTL can be configured via `-blocks-storage.bucket-store.metadata-cache.block-index-attributes-ttl`. #3629
* [ENHANCEMENT] Alertmanager: Add support for Azure blob storage. #3634
* [ENHANCEMENT] Compactor: tenants marked for deletion will now be fully cleaned up after some delay since deletion of last block. Cleanup includes removal of remaining marker files (including tenant deletion mark file) and files under `debug/metas`. #3613
* [ENHANCEMENT] Compactor: retry compaction of a single tenant on failure instead of re-running compaction for all tenants. #3627
* [ENHANCEMENT] Querier: Implement result caching for tenant query federation. #3640
* [ENHANCEMENT] API: Add a `mode` query parameter for the config endpoint: #3645
  * `/config?mode=diff`: Shows the YAML configuration with all values that differ from the defaults.
  * `/config?mode=defaults`: Shows the YAML configuration with all the default values.
* [ENHANCEMENT] OpenStack Swift: added the following config options to OpenStack Swift backend client: #3660
  - Chunks storage: `-swift.auth-version`, `-swift.max-retries`, `-swift.connect-timeout`, `-swift.request-timeout`.
  - Blocks storage: ` -blocks-storage.swift.auth-version`, ` -blocks-storage.swift.max-retries`, ` -blocks-storage.swift.connect-timeout`, ` -blocks-storage.swift.request-timeout`.
  - Ruler: `-ruler.storage.swift.auth-version`, `-ruler.storage.swift.max-retries`, `-ruler.storage.swift.connect-timeout`, `-ruler.storage.swift.request-timeout`.
* [ENHANCEMENT] Disabled in-memory shuffle-sharding subring cache in the store-gateway, ruler and compactor. This should reduce the memory utilisation in these services when shuffle-sharding is enabled, without introducing a significantly increase CPU utilisation. #3601
* [ENHANCEMENT] Shuffle sharding: optimised subring generation used by shuffle sharding. #3601
* [ENHANCEMENT] New /runtime_config endpoint that returns the defined runtime configuration in YAML format. The returned configuration includes overrides. #3639
* [ENHANCEMENT] Query-frontend: included the parameter name failed to validate in HTTP 400 message. #3703
* [ENHANCEMENT] Fail to startup Cortex if provided runtime config is invalid. #3707
* [ENHANCEMENT] Alertmanager: Add flags to customize the cluster configuration: #3667
  * `-alertmanager.cluster.gossip-interval`: The interval between sending gossip messages. By lowering this value (more frequent) gossip messages are propagated across cluster more quickly at the expense of increased bandwidth usage.
  * `-alertmanager.cluster.push-pull-interval`: The interval between gossip state syncs. Setting this interval lower (more frequent) will increase convergence speeds across larger clusters at the expense of increased bandwidth usage.
* [ENHANCEMENT] Distributor: change the error message returned when a received series has too many label values. The new message format has the series at the end and this plays better with Prometheus logs truncation. #3718
  - From: `sample for '<series>' has <value> label names; limit <value>`
  - To: `series has too many labels (actual: <value>, limit: <value>) series: '<series>'`
* [ENHANCEMENT] Improve bucket index loader to handle edge case where new tenant has not had blocks uploaded to storage yet. #3717
* [BUGFIX] Allow `-querier.max-query-lookback` use `y|w|d` suffix like deprecated `-store.max-look-back-period`. #3598
* [BUGFIX] Memberlist: Entry in the ring should now not appear again after using "Forget" feature (unless it's still heartbeating). #3603
* [BUGFIX] Ingester: do not close idle TSDBs while blocks shipping is in progress. #3630 #3632
* [BUGFIX] Ingester: correctly update `cortex_ingester_memory_users` and `cortex_ingester_active_series` when a tenant's idle TSDB is closed, when running Cortex with the blocks storage. #3646
* [BUGFIX] Querier: fix default value incorrectly overriding `-querier.frontend-address` in single-binary mode. #3650
* [BUGFIX] Compactor: delete `deletion-mark.json` at last when deleting a block in order to not leave partial blocks without deletion mark in the bucket if the compactor is interrupted while deleting a block. #3660
* [BUGFIX] Blocks storage: do not cleanup a partially uploaded block when `meta.json` upload fails. Despite failure to upload `meta.json`, this file may in some cases still appear in the bucket later. By skipping early cleanup, we avoid having corrupted blocks in the storage. #3660
* [BUGFIX] Alertmanager: disable access to `/alertmanager/metrics` (which exposes all Cortex metrics), `/alertmanager/-/reload` and `/alertmanager/debug/*`, which were available to any authenticated user with enabled AlertManager. #3678
* [BUGFIX] Query-Frontend: avoid creating many small sub-queries by discarding cache extents under 5 minutes #3653
* [BUGFIX] Ruler: Ensure the stale markers generated for evaluated rules respect the configured `-ruler.evaluation-delay-duration`. This will avoid issues with samples with NaN be persisted with timestamps set ahead of the next rule evaluation. #3687
* [BUGFIX] Alertmanager: don't serve HTTP requests until Alertmanager has fully started. Serving HTTP requests earlier may result in loss of configuration for the user. #3679
* [BUGFIX] Do not log "failed to load config" if runtime config file is empty. #3706
* [BUGFIX] Do not allow to use a runtime config file containing multiple YAML documents. #3706
* [BUGFIX] HA Tracker: don't track as error in the `cortex_kv_request_duration_seconds` metric a CAS operation intentionally aborted. #3745

## 1.6.0 / 2020-12-29

* [CHANGE] Query Frontend: deprecate `-querier.compress-http-responses` in favour of `-api.response-compression-enabled`. #3544
* [CHANGE] Querier: deprecated `-store.max-look-back-period`. You should use `-querier.max-query-lookback` instead. #3452
* [CHANGE] Blocks storage: increased `-blocks-storage.bucket-store.chunks-cache.attributes-ttl` default from `24h` to `168h` (1 week). #3528
* [CHANGE] Blocks storage: the config option `-blocks-storage.bucket-store.index-cache.postings-compression-enabled` has been deprecated and postings compression is always enabled. #3538
* [CHANGE] Ruler: gRPC message size default limits on the Ruler-client side have changed: #3523
  - limit for outgoing gRPC messages has changed from 2147483647 to 16777216 bytes
  - limit for incoming gRPC messages has changed from 4194304 to 104857600 bytes
* [FEATURE] Distributor/Ingester: Provide ability to not overflow writes in the presence of a leaving or unhealthy ingester. This allows for more efficient ingester rolling restarts. #3305
* [FEATURE] Query-frontend: introduced query statistics logged in the query-frontend when enabled via `-frontend.query-stats-enabled=true`. When enabled, the metric `cortex_query_seconds_total` is tracked, counting the sum of the wall time spent across all queriers while running queries (on a per-tenant basis). The metrics `cortex_request_duration_seconds` and `cortex_query_seconds_total` are different: the first one tracks the request duration (eg. HTTP request from the client), while the latter tracks the sum of the wall time on all queriers involved executing the query. #3539
* [ENHANCEMENT] API: Add GZIP HTTP compression to the API responses. Compression can be enabled via `-api.response-compression-enabled`. #3536
* [ENHANCEMENT] Added zone-awareness support on queries. When zone-awareness is enabled, queries will still succeed if all ingesters in a single zone will fail. #3414
* [ENHANCEMENT] Blocks storage ingester: exported more TSDB-related metrics. #3412
  - `cortex_ingester_tsdb_wal_corruptions_total`
  - `cortex_ingester_tsdb_head_truncations_failed_total`
  - `cortex_ingester_tsdb_head_truncations_total`
  - `cortex_ingester_tsdb_head_gc_duration_seconds`
* [ENHANCEMENT] Enforced keepalive on all gRPC clients used for inter-service communication. #3431
* [ENHANCEMENT] Added `cortex_alertmanager_config_hash` metric to expose hash of Alertmanager Config loaded per user. #3388
* [ENHANCEMENT] Query-Frontend / Query-Scheduler: New component called "Query-Scheduler" has been introduced. Query-Scheduler is simply a queue of requests, moved outside of Query-Frontend. This allows Query-Frontend to be scaled separately from number of queues. To make Query-Frontend and Querier use Query-Scheduler, they need to be started with `-frontend.scheduler-address` and `-querier.scheduler-address` options respectively. #3374 #3471
* [ENHANCEMENT] Query-frontend / Querier / Ruler: added `-querier.max-query-lookback` to limit how long back data (series and metadata) can be queried. This setting can be overridden on a per-tenant basis and is enforced in the query-frontend, querier and ruler. #3452 #3458
* [ENHANCEMENT] Querier: added `-querier.query-store-for-labels-enabled` to query store for label names, label values and series APIs. Only works with blocks storage engine. #3461 #3520
* [ENHANCEMENT] Ingester: exposed `-blocks-storage.tsdb.wal-segment-size-bytes` config option to customise the TSDB WAL segment max size. #3476
* [ENHANCEMENT] Compactor: concurrently run blocks cleaner for multiple tenants. Concurrency can be configured via `-compactor.cleanup-concurrency`. #3483
* [ENHANCEMENT] Compactor: shuffle tenants before running compaction. #3483
* [ENHANCEMENT] Compactor: wait for a stable ring at startup, when sharding is enabled. #3484
* [ENHANCEMENT] Store-gateway: added `-blocks-storage.bucket-store.index-header-lazy-loading-enabled` to enable index-header lazy loading (experimental). When enabled, index-headers will be mmap-ed only once required by a query and will be automatically released after `-blocks-storage.bucket-store.index-header-lazy-loading-idle-timeout` time of inactivity. #3498
* [ENHANCEMENT] Alertmanager: added metrics `cortex_alertmanager_notification_requests_total` and `cortex_alertmanager_notification_requests_failed_total`. #3518
* [ENHANCEMENT] Ingester: added `-blocks-storage.tsdb.head-chunks-write-buffer-size-bytes` to fine-tune the TSDB head chunks write buffer size when running Cortex blocks storage. #3518
* [ENHANCEMENT] /metrics now supports OpenMetrics output. HTTP and gRPC servers metrics can now include exemplars. #3524
* [ENHANCEMENT] Expose gRPC keepalive policy options by gRPC server. #3524
* [ENHANCEMENT] Blocks storage: enabled caching of `meta.json` attributes, configurable via `-blocks-storage.bucket-store.metadata-cache.metafile-attributes-ttl`. #3528
* [ENHANCEMENT] Compactor: added a config validation check to fail fast if the compactor has been configured invalid block range periods (each period is expected to be a multiple of the previous one). #3534
* [ENHANCEMENT] Blocks storage: concurrently fetch deletion marks from object storage. #3538
* [ENHANCEMENT] Blocks storage ingester: ingester can now close idle TSDB and delete local data. #3491 #3552
* [ENHANCEMENT] Blocks storage: add option to use V2 signatures for S3 authentication. #3540
* [ENHANCEMENT] Exported process metrics to monitor the number of memory map areas allocated. #3537
  * - `process_memory_map_areas`
  * - `process_memory_map_areas_limit`
* [ENHANCEMENT] Ruler: Expose gRPC client options. #3523
* [ENHANCEMENT] Compactor: added metrics to track on-going compaction. #3535
  * `cortex_compactor_tenants_discovered`
  * `cortex_compactor_tenants_skipped`
  * `cortex_compactor_tenants_processing_succeeded`
  * `cortex_compactor_tenants_processing_failed`
* [ENHANCEMENT] Added new experimental API endpoints: `POST /purger/delete_tenant` and `GET /purger/delete_tenant_status` for deleting all tenant data. Only works with blocks storage. Compactor removes blocks that belong to user marked for deletion. #3549 #3558
* [ENHANCEMENT] Chunks storage: add option to use V2 signatures for S3 authentication. #3560
* [ENHANCEMENT] HA Tracker: Added new limit `ha_max_clusters` to set the max number of clusters tracked for single user. This limit is disabled by default. #3668
* [BUGFIX] Query-Frontend: `cortex_query_seconds_total` now return seconds not nanoseconds. #3589
* [BUGFIX] Blocks storage ingester: fixed some cases leading to a TSDB WAL corruption after a partial write to disk. #3423
* [BUGFIX] Blocks storage: Fix the race between ingestion and `/flush` call resulting in overlapping blocks. #3422
* [BUGFIX] Querier: fixed `-querier.max-query-into-future` which wasn't correctly enforced on range queries. #3452
* [BUGFIX] Fixed float64 precision stability when aggregating metrics before exposing them. This could have lead to false counters resets when querying some metrics exposed by Cortex. #3506
* [BUGFIX] Querier: the meta.json sync concurrency done when running Cortex with the blocks storage is now controlled by `-blocks-storage.bucket-store.meta-sync-concurrency` instead of the incorrect `-blocks-storage.bucket-store.block-sync-concurrency` (default values are the same). #3531
* [BUGFIX] Querier: fixed initialization order of querier module when using blocks storage. It now (again) waits until blocks have been synchronized. #3551

## Blocksconvert

* [ENHANCEMENT] Scheduler: ability to ignore users based on regexp, using `-scheduler.ignore-users-regex` flag. #3477
* [ENHANCEMENT] Builder: Parallelize reading chunks in the final stage of building block. #3470
* [ENHANCEMENT] Builder: remove duplicate label names from chunk. #3547

## 1.5.0 / 2020-11-09

### Cortex

* [CHANGE] Blocks storage: update the default HTTP configuration values for the S3 client to the upstream Thanos default values. #3244
  - `-blocks-storage.s3.http.idle-conn-timeout` is set 90 seconds.
  - `-blocks-storage.s3.http.response-header-timeout` is set to 2 minutes.
* [CHANGE] Improved shuffle sharding support in the write path. This work introduced some config changes: #3090
  * Introduced `-distributor.sharding-strategy` CLI flag (and its respective `sharding_strategy` YAML config option) to explicitly specify which sharding strategy should be used in the write path
  * `-experimental.distributor.user-subring-size` flag renamed to `-distributor.ingestion-tenant-shard-size`
  * `user_subring_size` limit YAML config option renamed to `ingestion_tenant_shard_size`
* [CHANGE] Dropped "blank Alertmanager configuration; using fallback" message from Info to Debug level. #3205
* [CHANGE] Zone-awareness replication for time-series now should be explicitly enabled in the distributor via the `-distributor.zone-awareness-enabled` CLI flag (or its respective YAML config option). Before, zone-aware replication was implicitly enabled if a zone was set on ingesters. #3200
* [CHANGE] Removed the deprecated CLI flag `-config-yaml`. You should use `-schema-config-file` instead. #3225
* [CHANGE] Enforced the HTTP method required by some API endpoints which did (incorrectly) allow any method before that. #3228
  - `GET /`
  - `GET /config`
  - `GET /debug/fgprof`
  - `GET /distributor/all_user_stats`
  - `GET /distributor/ha_tracker`
  - `GET /all_user_stats`
  - `GET /ha-tracker`
  - `GET /api/v1/user_stats`
  - `GET /api/v1/chunks`
  - `GET <legacy-http-prefix>/user_stats`
  - `GET <legacy-http-prefix>/chunks`
  - `GET /services`
  - `GET /multitenant_alertmanager/status`
  - `GET /status` (alertmanager microservice)
  - `GET|POST /ingester/ring`
  - `GET|POST /ring`
  - `GET|POST /store-gateway/ring`
  - `GET|POST /compactor/ring`
  - `GET|POST /ingester/flush`
  - `GET|POST /ingester/shutdown`
  - `GET|POST /flush`
  - `GET|POST /shutdown`
  - `GET|POST /ruler/ring`
  - `POST /api/v1/push`
  - `POST <legacy-http-prefix>/push`
  - `POST /push`
  - `POST /ingester/push`
* [CHANGE] Renamed CLI flags to configure the network interface names from which automatically detect the instance IP. #3295
  - `-compactor.ring.instance-interface` renamed to `-compactor.ring.instance-interface-names`
  - `-store-gateway.sharding-ring.instance-interface` renamed to `-store-gateway.sharding-ring.instance-interface-names`
  - `-distributor.ring.instance-interface` renamed to `-distributor.ring.instance-interface-names`
  - `-ruler.ring.instance-interface` renamed to `-ruler.ring.instance-interface-names`
* [CHANGE] Renamed `-<prefix>.redis.enable-tls` CLI flag to `-<prefix>.redis.tls-enabled`, and its respective YAML config option from `enable_tls` to `tls_enabled`. #3298
* [CHANGE] Increased default `-<prefix>.redis.timeout` from `100ms` to `500ms`. #3301
* [CHANGE] `cortex_alertmanager_config_invalid` has been removed in favor of `cortex_alertmanager_config_last_reload_successful`. #3289
* [CHANGE] Query-frontend: POST requests whose body size exceeds 10MiB will be rejected. The max body size can be customised via `-frontend.max-body-size`. #3276
* [FEATURE] Shuffle sharding: added support for shuffle-sharding queriers in the query-frontend. When configured (`-frontend.max-queriers-per-tenant` globally, or using per-tenant limit `max_queriers_per_tenant`), each tenants's requests will be handled by different set of queriers. #3113 #3257
* [FEATURE] Shuffle sharding: added support for shuffle-sharding ingesters on the read path. When ingesters shuffle-sharding is enabled and `-querier.shuffle-sharding-ingesters-lookback-period` is set, queriers will fetch in-memory series from the minimum set of required ingesters, selecting only ingesters which may have received series since 'now - lookback period'. #3252
* [FEATURE] Query-frontend: added `compression` config to support results cache with compression. #3217
* [FEATURE] Add OpenStack Swift support to blocks storage. #3303
* [FEATURE] Added support for applying Prometheus relabel configs on series received by the distributor. A `metric_relabel_configs` field has been added to the per-tenant limits configuration. #3329
* [FEATURE] Support for Cassandra client SSL certificates. #3384
* [ENHANCEMENT] Ruler: Introduces two new limits `-ruler.max-rules-per-rule-group` and `-ruler.max-rule-groups-per-tenant` to control the number of rules per rule group and the total number of rule groups for a given user. They are disabled by default. #3366
* [ENHANCEMENT] Allow to specify multiple comma-separated Cortex services to `-target` CLI option (or its respective YAML config option). For example, `-target=all,compactor` can be used to start Cortex single-binary with compactor as well. #3275
* [ENHANCEMENT] Expose additional HTTP configs for the S3 backend client. New flag are listed below: #3244
  - `-blocks-storage.s3.http.idle-conn-timeout`
  - `-blocks-storage.s3.http.response-header-timeout`
  - `-blocks-storage.s3.http.insecure-skip-verify`
* [ENHANCEMENT] Added `cortex_query_frontend_connected_clients` metric to show the number of workers currently connected to the frontend. #3207
* [ENHANCEMENT] Shuffle sharding: improved shuffle sharding in the write path. Shuffle sharding now should be explicitly enabled via `-distributor.sharding-strategy` CLI flag (or its respective YAML config option) and guarantees stability, consistency, shuffling and balanced zone-awareness properties. #3090 #3214
* [ENHANCEMENT] Ingester: added new metric `cortex_ingester_active_series` to track active series more accurately. Also added options to control whether active series tracking is enabled (`-ingester.active-series-metrics-enabled`, defaults to false), and how often this metric is updated (`-ingester.active-series-metrics-update-period`) and max idle time for series to be considered inactive (`-ingester.active-series-metrics-idle-timeout`). #3153
* [ENHANCEMENT] Store-gateway: added zone-aware replication support to blocks replication in the store-gateway. #3200
* [ENHANCEMENT] Store-gateway: exported new metrics. #3231
  - `cortex_bucket_store_cached_series_fetch_duration_seconds`
  - `cortex_bucket_store_cached_postings_fetch_duration_seconds`
  - `cortex_bucket_stores_gate_queries_max`
* [ENHANCEMENT] Added `-version` flag to Cortex. #3233
* [ENHANCEMENT] Hash ring: added instance registered timestamp to the ring. #3248
* [ENHANCEMENT] Reduce tail latency by smoothing out spikes in rate of chunk flush operations. #3191
* [ENHANCEMENT] User Cortex as User Agent in http requests issued by Configs DB client. #3264
* [ENHANCEMENT] Experimental Ruler API: Fetch rule groups from object storage in parallel. #3218
* [ENHANCEMENT] Chunks GCS object storage client uses the `fields` selector to limit the payload size when listing objects in the bucket. #3218 #3292
* [ENHANCEMENT] Added shuffle sharding support to ruler. Added new metric `cortex_ruler_sync_rules_total`. #3235
* [ENHANCEMENT] Return an explicit error when the store-gateway is explicitly requested without a blocks storage engine. #3287
* [ENHANCEMENT] Ruler: only load rules that belong to the ruler. Improves rules syncing performances when ruler sharding is enabled. #3269
* [ENHANCEMENT] Added `-<prefix>.redis.tls-insecure-skip-verify` flag. #3298
* [ENHANCEMENT] Added `cortex_alertmanager_config_last_reload_successful_seconds` metric to show timestamp of last successful AM config reload. #3289
* [ENHANCEMENT] Blocks storage: reduced number of bucket listing operations to list block content (applies to newly created blocks only). #3363
* [ENHANCEMENT] Ruler: Include the tenant ID on the notifier logs. #3372
* [ENHANCEMENT] Blocks storage Compactor: Added `-compactor.enabled-tenants` and `-compactor.disabled-tenants` to explicitly enable or disable compaction of specific tenants. #3385
* [ENHANCEMENT] Blocks storage ingester: Creating checkpoint only once even when there are multiple Head compactions in a single `Compact()` call. #3373
* [BUGFIX] Blocks storage ingester: Read repair memory-mapped chunks file which can end up being empty on abrupt shutdowns combined with faulty disks. #3373
* [BUGFIX] Blocks storage ingester: Close TSDB resources on failed startup preventing ingester OOMing. #3373
* [BUGFIX] No-longer-needed ingester operations for queries triggered by queriers and rulers are now canceled. #3178
* [BUGFIX] Ruler: directories in the configured `rules-path` will be removed on startup and shutdown in order to ensure they don't persist between runs. #3195
* [BUGFIX] Handle hash-collisions in the query path. #3192
* [BUGFIX] Check for postgres rows errors. #3197
* [BUGFIX] Ruler Experimental API: Don't allow rule groups without names or empty rule groups. #3210
* [BUGFIX] Experimental Alertmanager API: Do not allow empty Alertmanager configurations or bad template filenames to be submitted through the configuration API. #3185
* [BUGFIX] Reduce failures to update heartbeat when using Consul. #3259
* [BUGFIX] When using ruler sharding, moving all user rule groups from ruler to a different one and then back could end up with some user groups not being evaluated at all. #3235
* [BUGFIX] Fixed shuffle sharding consistency when zone-awareness is enabled and the shard size is increased or instances in a new zone are added. #3299
* [BUGFIX] Use a valid grpc header when logging IP addresses. #3307
* [BUGFIX] Fixed the metric `cortex_prometheus_rule_group_duration_seconds` in the Ruler, it wouldn't report any values. #3310
* [BUGFIX] Fixed gRPC connections leaking in rulers when rulers sharding is enabled and APIs called. #3314
* [BUGFIX] Fixed shuffle sharding consistency when zone-awareness is enabled and the shard size is increased or instances in a new zone are added. #3299
* [BUGFIX] Fixed Gossip memberlist members joining when addresses are configured using DNS-based service discovery. #3360
* [BUGFIX] Ingester: fail to start an ingester running the blocks storage, if unable to load any existing TSDB at startup. #3354
* [BUGFIX] Blocks storage: Avoid deletion of blocks in the ingester which are not shipped to the storage yet. #3346
* [BUGFIX] Fix common prefixes returned by List method of S3 client. #3358
* [BUGFIX] Honor configured timeout in Azure and GCS object clients. #3285
* [BUGFIX] Blocks storage: Avoid creating blocks larger than configured block range period on forced compaction and when TSDB is idle. #3344
* [BUGFIX] Shuffle sharding: fixed max global series per user/metric limit when shuffle sharding and `-distributor.shard-by-all-labels=true` are both enabled in distributor. When using these global limits you should now set `-distributor.sharding-strategy` and `-distributor.zone-awareness-enabled` to ingesters too. #3369
* [BUGFIX] Slow query logging: when using downstream server request parameters were not logged. #3276
* [BUGFIX] Fixed tenant detection in the ruler and alertmanager API when running without auth. #3343

### Blocksconvert

* [ENHANCEMENT] Blocksconvert – Builder: download plan file locally before processing it. #3209
* [ENHANCEMENT] Blocksconvert – Cleaner: added new tool for deleting chunks data. #3283
* [ENHANCEMENT] Blocksconvert – Scanner: support for scanning specific date-range only. #3222
* [ENHANCEMENT] Blocksconvert – Scanner: metrics for tracking progress. #3222
* [ENHANCEMENT] Blocksconvert – Builder: retry block upload before giving up. #3245
* [ENHANCEMENT] Blocksconvert – Scanner: upload plans concurrently. #3340
* [BUGFIX] Blocksconvert: fix chunks ordering in the block. Chunks in different order than series work just fine in TSDB blocks at the moment, but it's not consistent with what Prometheus does and future Prometheus and Cortex optimizations may rely on this ordering. #3371

## 1.4.0 / 2020-10-02

* [CHANGE] TLS configuration for gRPC, HTTP and etcd clients is now marked as experimental. These features are not yet fully baked, and we expect possible small breaking changes in Cortex 1.5. #3198
* [CHANGE] Cassandra backend support is now GA (stable). #3180
* [CHANGE] Blocks storage is now GA (stable). The `-experimental` prefix has been removed from all CLI flags related to the blocks storage (no YAML config changes). #3180 #3201
  - `-experimental.blocks-storage.*` flags renamed to `-blocks-storage.*`
  - `-experimental.store-gateway.*` flags renamed to `-store-gateway.*`
  - `-experimental.querier.store-gateway-client.*` flags renamed to `-querier.store-gateway-client.*`
  - `-experimental.querier.store-gateway-addresses` flag renamed to `-querier.store-gateway-addresses`
  - `-store-gateway.replication-factor` flag renamed to `-store-gateway.sharding-ring.replication-factor`
  - `-store-gateway.tokens-file-path` flag renamed to `store-gateway.sharding-ring.tokens-file-path`
* [CHANGE] Ingester: Removed deprecated untyped record from chunks WAL. Only if you are running `v1.0` or below, it is recommended to first upgrade to `v1.1`/`v1.2`/`v1.3` and run it for a day before upgrading to `v1.4` to avoid data loss. #3115
* [CHANGE] Distributor API endpoints are no longer served unless target is set to `distributor` or `all`. #3112
* [CHANGE] Increase the default Cassandra client replication factor to 3. #3007
* [CHANGE] Blocks storage: removed the support to transfer blocks between ingesters on shutdown. When running the Cortex blocks storage, ingesters are expected to run with a persistent disk. The following metrics have been removed: #2996
  * `cortex_ingester_sent_files`
  * `cortex_ingester_received_files`
  * `cortex_ingester_received_bytes_total`
  * `cortex_ingester_sent_bytes_total`
* [CHANGE] The buckets for the `cortex_chunk_store_index_lookups_per_query` metric have been changed to 1, 2, 4, 8, 16. #3021
* [CHANGE] Blocks storage: the `operation` label value `getrange` has changed into `get_range` for the metrics `thanos_store_bucket_cache_operation_requests_total` and `thanos_store_bucket_cache_operation_hits_total`. #3000
* [CHANGE] Experimental Delete Series: `/api/v1/admin/tsdb/delete_series` and `/api/v1/admin/tsdb/cancel_delete_request` purger APIs to return status code `204` instead of `200` for success. #2946
* [CHANGE] Histogram `cortex_memcache_request_duration_seconds` `method` label value changes from `Memcached.Get` to `Memcached.GetBatched` for batched lookups, and is not reported for non-batched lookups (label value `Memcached.GetMulti` remains, and had exactly the same value as `Get` in nonbatched lookups).  The same change applies to tracing spans. #3046
* [CHANGE] TLS server validation is now enabled by default, a new parameter `tls_insecure_skip_verify` can be set to true to skip validation optionally. #3030
* [CHANGE] `cortex_ruler_config_update_failures_total` has been removed in favor of `cortex_ruler_config_last_reload_successful`. #3056
* [CHANGE] `ruler.evaluation_delay_duration` field in YAML config has been moved and renamed to `limits.ruler_evaluation_delay_duration`. #3098
* [CHANGE] Removed obsolete `results_cache.max_freshness` from YAML config (deprecated since Cortex 1.2). #3145
* [CHANGE] Removed obsolete `-promql.lookback-delta` option (deprecated since Cortex 1.2, replaced with `-querier.lookback-delta`). #3144
* [CHANGE] Cache: added support for Redis Cluster and Redis Sentinel. #2961
  - The following changes have been made in Redis configuration:
   - `-redis.master_name` added
   - `-redis.db` added
   - `-redis.max-active-conns` changed to `-redis.pool-size`
   - `-redis.max-conn-lifetime` changed to `-redis.max-connection-age`
   - `-redis.max-idle-conns` removed
   - `-redis.wait-on-pool-exhaustion` removed
* [CHANGE] TLS configuration for gRPC, HTTP and etcd clients is now marked as experimental. These features are not yet fully baked, and we expect possible small breaking changes in Cortex 1.5. #3198
* [CHANGE] Fixed store-gateway CLI flags inconsistencies. #3201
  - `-store-gateway.replication-factor` flag renamed to `-store-gateway.sharding-ring.replication-factor`
  - `-store-gateway.tokens-file-path` flag renamed to `store-gateway.sharding-ring.tokens-file-path`
* [FEATURE] Logging of the source IP passed along by a reverse proxy is now supported by setting the `-server.log-source-ips-enabled`. For non standard headers the settings `-server.log-source-ips-header` and `-server.log-source-ips-regex` can be used. #2985
* [FEATURE] Blocks storage: added shuffle sharding support to store-gateway blocks sharding. Added the following additional metrics to store-gateway: #3069
  * `cortex_bucket_stores_tenants_discovered`
  * `cortex_bucket_stores_tenants_synced`
* [FEATURE] Experimental blocksconvert: introduce an experimental tool `blocksconvert` to migrate long-term storage chunks to blocks. #3092 #3122 #3127 #3162
* [ENHANCEMENT] Improve the Alertmanager logging when serving requests from its API / UI. #3397
* [ENHANCEMENT] Add support for azure storage in China, German and US Government environments. #2988
* [ENHANCEMENT] Query-tee: added a small tolerance to floating point sample values comparison. #2994
* [ENHANCEMENT] Query-tee: add support for doing a passthrough of requests to preferred backend for unregistered routes #3018
* [ENHANCEMENT] Expose `storage.aws.dynamodb.backoff_config` configuration file field. #3026
* [ENHANCEMENT] Added `cortex_request_message_bytes` and `cortex_response_message_bytes` histograms to track received and sent gRPC message and HTTP request/response sizes. Added `cortex_inflight_requests` gauge to track number of inflight gRPC and HTTP requests. #3064
* [ENHANCEMENT] Publish ruler's ring metrics. #3074
* [ENHANCEMENT] Add config validation to the experimental Alertmanager API. Invalid configs are no longer accepted. #3053
* [ENHANCEMENT] Add "integration" as a label for `cortex_alertmanager_notifications_total` and `cortex_alertmanager_notifications_failed_total` metrics. #3056
* [ENHANCEMENT] Add `cortex_ruler_config_last_reload_successful` and `cortex_ruler_config_last_reload_successful_seconds` to check status of users rule manager. #3056
* [ENHANCEMENT] The configuration validation now fails if an empty YAML node has been set for a root YAML config property. #3080
* [ENHANCEMENT] Memcached dial() calls now have a circuit-breaker to avoid hammering a broken cache. #3051, #3189
* [ENHANCEMENT] `-ruler.evaluation-delay-duration` is now overridable as a per-tenant limit, `ruler_evaluation_delay_duration`. #3098
* [ENHANCEMENT] Add TLS support to etcd client. #3102
* [ENHANCEMENT] When a tenant accesses the Alertmanager UI or its API, if we have valid `-alertmanager.configs.fallback` we'll use that to start the manager and avoid failing the request. #3073
* [ENHANCEMENT] Add `DELETE api/v1/rules/{namespace}` to the Ruler. It allows all the rule groups of a namespace to be deleted. #3120
* [ENHANCEMENT] Experimental Delete Series: Retry processing of Delete requests during failures. #2926
* [ENHANCEMENT] Improve performance of QueryStream() in ingesters. #3177
* [ENHANCEMENT] Modules included in "All" target are now visible in output of `-modules` CLI flag. #3155
* [ENHANCEMENT] Added `/debug/fgprof` endpoint to debug running Cortex process using `fgprof`. This adds up to the existing `/debug/...` endpoints. #3131
* [ENHANCEMENT] Blocks storage: optimised `/api/v1/series` for blocks storage. (#2976)
* [BUGFIX] Ruler: when loading rules from "local" storage, check for directory after resolving symlink. #3137
* [BUGFIX] Query-frontend: Fixed rounding for incoming query timestamps, to be 100% Prometheus compatible. #2990
* [BUGFIX] Querier: Merge results from chunks and blocks ingesters when using streaming of results. #3013
* [BUGFIX] Querier: query /series from ingesters regardless the `-querier.query-ingesters-within` setting. #3035
* [BUGFIX] Blocks storage: Ingester is less likely to hit gRPC message size limit when streaming data to queriers. #3015
* [BUGFIX] Blocks storage: fixed memberlist support for the store-gateways and compactors ring used when blocks sharding is enabled. #3058 #3095
* [BUGFIX] Fix configuration for TLS server validation, TLS skip verify was hardcoded to true for all TLS configurations and prevented validation of server certificates. #3030
* [BUGFIX] Fixes the Alertmanager panicking when no `-alertmanager.web.external-url` is provided. #3017
* [BUGFIX] Fixes the registration of the Alertmanager API metrics `cortex_alertmanager_alerts_received_total` and `cortex_alertmanager_alerts_invalid_total`. #3065
* [BUGFIX] Fixes `flag needs an argument: -config.expand-env` error. #3087
* [BUGFIX] An index optimisation actually slows things down when using caching. Moved it to the right location. #2973
* [BUGFIX] Ingester: If push request contained both valid and invalid samples, valid samples were ingested but not stored to WAL of the chunks storage. This has been fixed. #3067
* [BUGFIX] Cassandra: fixed consistency setting in the CQL session when creating the keyspace. #3105
* [BUGFIX] Ruler: Config API would return both the `record` and `alert` in `YAML` response keys even when one of them must be empty. #3120
* [BUGFIX] Index page now uses configured HTTP path prefix when creating links. #3126
* [BUGFIX] Purger: fixed deadlock when reloading of tombstones failed. #3182
* [BUGFIX] Fixed panic in flusher job, when error writing chunks to the store would cause "idle" chunks to be flushed, which triggered panic. #3140
* [BUGFIX] Index page no longer shows links that are not valid for running Cortex instance. #3133
* [BUGFIX] Configs: prevent validation of templates to fail when using template functions. #3157
* [BUGFIX] Configuring the S3 URL with an `@` but without username and password doesn't enable the AWS static credentials anymore. #3170
* [BUGFIX] Limit errors on ranged queries (`api/v1/query_range`) no longer return a status code `500` but `422` instead. #3167
* [BUGFIX] Handle hash-collisions in the query path. Before this fix, Cortex could occasionally mix up two different series in a query, leading to invalid results, when `-querier.ingester-streaming` was used. #3192

## 1.3.0 / 2020-08-21

* [CHANGE] Replace the metric `cortex_alertmanager_configs` with `cortex_alertmanager_config_invalid` exposed by Alertmanager. #2960
* [CHANGE] Experimental Delete Series: Change target flag for purger from `data-purger` to `purger`. #2777
* [CHANGE] Experimental blocks storage: The max concurrent queries against the long-term storage, configured via `-experimental.blocks-storage.bucket-store.max-concurrent`, is now a limit shared across all tenants and not a per-tenant limit anymore. The default value has changed from `20` to `100` and the following new metrics have been added: #2797
  * `cortex_bucket_stores_gate_queries_concurrent_max`
  * `cortex_bucket_stores_gate_queries_in_flight`
  * `cortex_bucket_stores_gate_duration_seconds`
* [CHANGE] Metric `cortex_ingester_flush_reasons` has been renamed to `cortex_ingester_flushing_enqueued_series_total`, and new metric `cortex_ingester_flushing_dequeued_series_total` with `outcome` label (superset of reason) has been added. #2802 #2818 #2998
* [CHANGE] Experimental Delete Series: Metric `cortex_purger_oldest_pending_delete_request_age_seconds` would track age of delete requests since they are over their cancellation period instead of their creation time. #2806
* [CHANGE] Experimental blocks storage: the store-gateway service is required in a Cortex cluster running with the experimental blocks storage. Removed the `-experimental.tsdb.store-gateway-enabled` CLI flag and `store_gateway_enabled` YAML config option. The store-gateway is now always enabled when the storage engine is `blocks`. #2822
* [CHANGE] Experimental blocks storage: removed support for `-experimental.blocks-storage.bucket-store.max-sample-count` flag because the implementation was flawed. To limit the number of samples/chunks processed by a single query you can set `-store.query-chunk-limit`, which is now supported by the blocks storage too. #2852
* [CHANGE] Ingester: Chunks flushed via /flush stay in memory until retention period is reached. This affects `cortex_ingester_memory_chunks` metric. #2778
* [CHANGE] Querier: the error message returned when the query time range exceeds `-store.max-query-length` has changed from `invalid query, length > limit (X > Y)` to `the query time range exceeds the limit (query length: X, limit: Y)`. #2826
* [CHANGE] Add `component` label to metrics exposed by chunk, delete and index store clients. #2774
* [CHANGE] Querier: when `-querier.query-ingesters-within` is configured, the time range of the query sent to ingesters is now manipulated to ensure the query start time is not older than 'now - query-ingesters-within'. #2904
* [CHANGE] KV: The `role` label which was a label of `multi` KV store client only has been added to metrics of every KV store client. If KV store client is not `multi`, then the value of `role` label is `primary`. #2837
* [CHANGE] Added the `engine` label to the metrics exposed by the Prometheus query engine, to distinguish between `ruler` and `querier` metrics. #2854
* [CHANGE] Added ruler to the single binary when started with `-target=all` (default). #2854
* [CHANGE] Experimental blocks storage: compact head when opening TSDB. This should only affect ingester startup after it was unable to compact head in previous run. #2870
* [CHANGE] Metric `cortex_overrides_last_reload_successful` has been renamed to `cortex_runtime_config_last_reload_successful`. #2874
* [CHANGE] HipChat support has been removed from the alertmanager (because removed from the Prometheus upstream too). #2902
* [CHANGE] Add constant label `name` to metric `cortex_cache_request_duration_seconds`. #2903
* [CHANGE] Add `user` label to metric `cortex_query_frontend_queue_length`. #2939
* [CHANGE] Experimental blocks storage: cleaned up the config and renamed "TSDB" to "blocks storage". #2937
  - The storage engine setting value has been changed from `tsdb` to `blocks`; this affects `-store.engine` CLI flag and its respective YAML option.
  - The root level YAML config has changed from `tsdb` to `blocks_storage`
  - The prefix of all CLI flags has changed from `-experimental.tsdb.` to `-experimental.blocks-storage.`
  - The following settings have been grouped under `tsdb` property in the YAML config and their CLI flags changed:
    - `-experimental.tsdb.dir` changed to `-experimental.blocks-storage.tsdb.dir`
    - `-experimental.tsdb.block-ranges-period` changed to `-experimental.blocks-storage.tsdb.block-ranges-period`
    - `-experimental.tsdb.retention-period` changed to `-experimental.blocks-storage.tsdb.retention-period`
    - `-experimental.tsdb.ship-interval` changed to `-experimental.blocks-storage.tsdb.ship-interval`
    - `-experimental.tsdb.ship-concurrency` changed to `-experimental.blocks-storage.tsdb.ship-concurrency`
    - `-experimental.tsdb.max-tsdb-opening-concurrency-on-startup` changed to `-experimental.blocks-storage.tsdb.max-tsdb-opening-concurrency-on-startup`
    - `-experimental.tsdb.head-compaction-interval` changed to `-experimental.blocks-storage.tsdb.head-compaction-interval`
    - `-experimental.tsdb.head-compaction-concurrency` changed to `-experimental.blocks-storage.tsdb.head-compaction-concurrency`
    - `-experimental.tsdb.head-compaction-idle-timeout` changed to `-experimental.blocks-storage.tsdb.head-compaction-idle-timeout`
    - `-experimental.tsdb.stripe-size` changed to `-experimental.blocks-storage.tsdb.stripe-size`
    - `-experimental.tsdb.wal-compression-enabled` changed to `-experimental.blocks-storage.tsdb.wal-compression-enabled`
    - `-experimental.tsdb.flush-blocks-on-shutdown` changed to `-experimental.blocks-storage.tsdb.flush-blocks-on-shutdown`
* [CHANGE] Flags `-bigtable.grpc-use-gzip-compression`, `-ingester.client.grpc-use-gzip-compression`, `-querier.frontend-client.grpc-use-gzip-compression` are now deprecated. #2940
* [CHANGE] Limit errors reported by ingester during query-time now return HTTP status code 422. #2941
* [FEATURE] Introduced `ruler.for-outage-tolerance`, Max time to tolerate outage for restoring "for" state of alert. #2783
* [FEATURE] Introduced `ruler.for-grace-period`, Minimum duration between alert and restored "for" state. This is maintained only for alerts with configured "for" time greater than grace period. #2783
* [FEATURE] Introduced `ruler.resend-delay`, Minimum amount of time to wait before resending an alert to Alertmanager. #2783
* [FEATURE] Ruler: added `local` filesystem support to store rules (read-only). #2854
* [ENHANCEMENT] Upgraded Docker base images to `alpine:3.12`. #2862
* [ENHANCEMENT] Experimental: Querier can now optionally query secondary store. This is specified by using `-querier.second-store-engine` option, with values `chunks` or `blocks`. Standard configuration options for this store are used. Additionally, this querying can be configured to happen only for queries that need data older than `-querier.use-second-store-before-time`. Default value of zero will always query secondary store. #2747
* [ENHANCEMENT] Query-tee: increased the `cortex_querytee_request_duration_seconds` metric buckets granularity. #2799
* [ENHANCEMENT] Query-tee: fail to start if the configured `-backend.preferred` is unknown. #2799
* [ENHANCEMENT] Ruler: Added the following metrics: #2786
  * `cortex_prometheus_notifications_latency_seconds`
  * `cortex_prometheus_notifications_errors_total`
  * `cortex_prometheus_notifications_sent_total`
  * `cortex_prometheus_notifications_dropped_total`
  * `cortex_prometheus_notifications_queue_length`
  * `cortex_prometheus_notifications_queue_capacity`
  * `cortex_prometheus_notifications_alertmanagers_discovered`
* [ENHANCEMENT] The behavior of the `/ready` was changed for the query frontend to indicate when it was ready to accept queries. This is intended for use by a read path load balancer that would want to wait for the frontend to have attached queriers before including it in the backend. #2733
* [ENHANCEMENT] Experimental Delete Series: Add support for deletion of chunks for remaining stores. #2801
* [ENHANCEMENT] Add `-modules` command line flag to list possible values for `-target`. Also, log warning if given target is internal component. #2752
* [ENHANCEMENT] Added `-ingester.flush-on-shutdown-with-wal-enabled` option to enable chunks flushing even when WAL is enabled. #2780
* [ENHANCEMENT] Query-tee: Support for custom API prefix by using `-server.path-prefix` option. #2814
* [ENHANCEMENT] Query-tee: Forward `X-Scope-OrgId` header to backend, if present in the request. #2815
* [ENHANCEMENT] Experimental blocks storage: Added `-experimental.blocks-storage.tsdb.head-compaction-idle-timeout` option to force compaction of data in memory into a block. #2803
* [ENHANCEMENT] Experimental blocks storage: Added support for flushing blocks via `/flush`, `/shutdown` (previously these only worked for chunks storage) and by using `-experimental.blocks-storage.tsdb.flush-blocks-on-shutdown` option. #2794
* [ENHANCEMENT] Experimental blocks storage: Added support to enforce max query time range length via `-store.max-query-length`. #2826
* [ENHANCEMENT] Experimental blocks storage: Added support to limit the max number of chunks that can be fetched from the long-term storage while executing a query. The limit is enforced both in the querier and store-gateway, and is configurable via `-store.query-chunk-limit`. #2852 #2922
* [ENHANCEMENT] Ingester: Added new metric `cortex_ingester_flush_series_in_progress` that reports number of ongoing flush-series operations. Useful when calling `/flush` handler: if `cortex_ingester_flush_queue_length + cortex_ingester_flush_series_in_progress` is 0, all flushes are finished. #2778
* [ENHANCEMENT] Memberlist members can join cluster via SRV records. #2788
* [ENHANCEMENT] Added configuration options for chunks s3 client. #2831
  * `s3.endpoint`
  * `s3.region`
  * `s3.access-key-id`
  * `s3.secret-access-key`
  * `s3.insecure`
  * `s3.sse-encryption`
  * `s3.http.idle-conn-timeout`
  * `s3.http.response-header-timeout`
  * `s3.http.insecure-skip-verify`
* [ENHANCEMENT] Prometheus upgraded. #2798 #2849 #2867 #2902 #2918
  * Optimized labels regex matchers for patterns containing literals (eg. `foo.*`, `.*foo`, `.*foo.*`)
* [ENHANCEMENT] Add metric `cortex_ruler_config_update_failures_total` to Ruler to track failures of loading rules files. #2857
* [ENHANCEMENT] Experimental Alertmanager: Alertmanager configuration persisted to object storage using an experimental API that accepts and returns YAML-based Alertmanager configuration. #2768
* [ENHANCEMENT] Ruler: `-ruler.alertmanager-url` now supports multiple URLs. Each URL is treated as a separate Alertmanager group. Support for multiple Alertmanagers in a group can be achieved by using DNS service discovery. #2851
* [ENHANCEMENT] Experimental blocks storage: Cortex Flusher now works with blocks engine. Flusher needs to be provided with blocks-engine configuration, existing Flusher flags are not used (they are only relevant for chunks engine). Note that flush errors are only reported via log. #2877
* [ENHANCEMENT] Flusher: Added `-flusher.exit-after-flush` option (defaults to true) to control whether Cortex should stop completely after Flusher has finished its work. #2877
* [ENHANCEMENT] Added metrics `cortex_config_hash` and `cortex_runtime_config_hash` to expose hash of the currently active config file. #2874
* [ENHANCEMENT] Logger: added JSON logging support, configured via the `-log.format=json` CLI flag or its respective YAML config option. #2386
* [ENHANCEMENT] Added new flags `-bigtable.grpc-compression`, `-ingester.client.grpc-compression`, `-querier.frontend-client.grpc-compression` to configure compression used by gRPC. Valid values are `gzip`, `snappy`, or empty string (no compression, default). #2940
* [ENHANCEMENT] Clarify limitations of the `/api/v1/series`, `/api/v1/labels` and `/api/v1/label/{name}/values` endpoints. #2953
* [ENHANCEMENT] Ingester: added `Dropped` outcome to metric `cortex_ingester_flushing_dequeued_series_total`. #2998
* [BUGFIX] Fixed a bug with `api/v1/query_range` where no responses would return null values for `result` and empty values for `resultType`. #2962
* [BUGFIX] Fixed a bug in the index intersect code causing storage to return more chunks/series than required. #2796
* [BUGFIX] Fixed the number of reported keys in the background cache queue. #2764
* [BUGFIX] Fix race in processing of headers in sharded queries. #2762
* [BUGFIX] Query Frontend: Do not re-split sharded requests around ingester boundaries. #2766
* [BUGFIX] Experimental Delete Series: Fixed a problem with cache generation numbers prefixed to cache keys. #2800
* [BUGFIX] Ingester: Flushing chunks via `/flush` endpoint could previously lead to panic, if chunks were already flushed before and then removed from memory during the flush caused by `/flush` handler. Immediate flush now doesn't cause chunks to be flushed again. Samples received during flush triggered via `/flush` handler are no longer discarded. #2778
* [BUGFIX] Prometheus upgraded. #2849
  * Fixed unknown symbol error during head compaction
* [BUGFIX] Fix panic when using cassandra as store for both index and delete requests. #2774
* [BUGFIX] Experimental Delete Series: Fixed a data race in Purger. #2817
* [BUGFIX] KV: Fixed a bug that triggered a panic due to metrics being registered with the same name but different labels when using a `multi` configured KV client. #2837
* [BUGFIX] Query-frontend: Fix passing HTTP `Host` header if `-frontend.downstream-url` is configured. #2880
* [BUGFIX] Ingester: Improve time-series distribution when `-experimental.distributor.user-subring-size` is enabled. #2887
* [BUGFIX] Set content type to `application/x-protobuf` for remote_read responses. #2915
* [BUGFIX] Fixed ruler and store-gateway instance registration in the ring (when sharding is enabled) when a new instance replaces abruptly terminated one, and the only difference between the two instances is the address. #2954
* [BUGFIX] Fixed `Missing chunks and index config causing silent failure` Absence of chunks and index from schema config is not validated. #2732
* [BUGFIX] Fix panic caused by KVs from boltdb being used beyond their life. #2971
* [BUGFIX] Experimental blocks storage: `/api/v1/series`, `/api/v1/labels` and `/api/v1/label/{name}/values` only query the TSDB head regardless of the configured `-experimental.blocks-storage.tsdb.retention-period`. #2974
* [BUGFIX] Ingester: Avoid indefinite checkpointing in case of surge in number of series. #2955
* [BUGFIX] Querier: query /series from ingesters regardless the `-querier.query-ingesters-within` setting. #3035
* [BUGFIX] Ruler: fixed an unintentional breaking change introduced in the ruler's `alertmanager_url` YAML config option, which changed the value from a string to a list of strings. #2989

## 1.2.0 / 2020-07-01

* [CHANGE] Metric `cortex_kv_request_duration_seconds` now includes `name` label to denote which client is being used as well as the `backend` label to denote the KV backend implementation in use. #2648
* [CHANGE] Experimental Ruler: Rule groups persisted to object storage using the experimental API have an updated object key encoding to better handle special characters. Rule groups previously-stored using object storage must be renamed to the new format. #2646
* [CHANGE] Query Frontend now uses Round Robin to choose a tenant queue to service next. #2553
* [CHANGE] `-promql.lookback-delta` is now deprecated and has been replaced by `-querier.lookback-delta` along with `lookback_delta` entry under `querier` in the config file. `-promql.lookback-delta` will be removed in v1.4.0. #2604
* [CHANGE] Experimental TSDB: removed `-experimental.tsdb.bucket-store.binary-index-header-enabled` flag. Now the binary index-header is always enabled.
* [CHANGE] Experimental TSDB: Renamed index-cache metrics to use original metric names from Thanos, as Cortex is not aggregating them in any way: #2627
  * `cortex_<service>_blocks_index_cache_items_evicted_total` => `thanos_store_index_cache_items_evicted_total{name="index-cache"}`
  * `cortex_<service>_blocks_index_cache_items_added_total` => `thanos_store_index_cache_items_added_total{name="index-cache"}`
  * `cortex_<service>_blocks_index_cache_requests_total` => `thanos_store_index_cache_requests_total{name="index-cache"}`
  * `cortex_<service>_blocks_index_cache_items_overflowed_total` => `thanos_store_index_cache_items_overflowed_total{name="index-cache"}`
  * `cortex_<service>_blocks_index_cache_hits_total` => `thanos_store_index_cache_hits_total{name="index-cache"}`
  * `cortex_<service>_blocks_index_cache_items` => `thanos_store_index_cache_items{name="index-cache"}`
  * `cortex_<service>_blocks_index_cache_items_size_bytes` => `thanos_store_index_cache_items_size_bytes{name="index-cache"}`
  * `cortex_<service>_blocks_index_cache_total_size_bytes` => `thanos_store_index_cache_total_size_bytes{name="index-cache"}`
  * `cortex_<service>_blocks_index_cache_memcached_operations_total` =>  `thanos_memcached_operations_total{name="index-cache"}`
  * `cortex_<service>_blocks_index_cache_memcached_operation_failures_total` =>  `thanos_memcached_operation_failures_total{name="index-cache"}`
  * `cortex_<service>_blocks_index_cache_memcached_operation_duration_seconds` =>  `thanos_memcached_operation_duration_seconds{name="index-cache"}`
  * `cortex_<service>_blocks_index_cache_memcached_operation_skipped_total` =>  `thanos_memcached_operation_skipped_total{name="index-cache"}`
* [CHANGE] Experimental TSDB: Renamed metrics in bucket stores: #2627
  * `cortex_<service>_blocks_meta_syncs_total` => `cortex_blocks_meta_syncs_total{component="<service>"}`
  * `cortex_<service>_blocks_meta_sync_failures_total` => `cortex_blocks_meta_sync_failures_total{component="<service>"}`
  * `cortex_<service>_blocks_meta_sync_duration_seconds` => `cortex_blocks_meta_sync_duration_seconds{component="<service>"}`
  * `cortex_<service>_blocks_meta_sync_consistency_delay_seconds` => `cortex_blocks_meta_sync_consistency_delay_seconds{component="<service>"}`
  * `cortex_<service>_blocks_meta_synced` => `cortex_blocks_meta_synced{component="<service>"}`
  * `cortex_<service>_bucket_store_block_loads_total` => `cortex_bucket_store_block_loads_total{component="<service>"}`
  * `cortex_<service>_bucket_store_block_load_failures_total` => `cortex_bucket_store_block_load_failures_total{component="<service>"}`
  * `cortex_<service>_bucket_store_block_drops_total` => `cortex_bucket_store_block_drops_total{component="<service>"}`
  * `cortex_<service>_bucket_store_block_drop_failures_total` => `cortex_bucket_store_block_drop_failures_total{component="<service>"}`
  * `cortex_<service>_bucket_store_blocks_loaded` => `cortex_bucket_store_blocks_loaded{component="<service>"}`
  * `cortex_<service>_bucket_store_series_data_touched` => `cortex_bucket_store_series_data_touched{component="<service>"}`
  * `cortex_<service>_bucket_store_series_data_fetched` => `cortex_bucket_store_series_data_fetched{component="<service>"}`
  * `cortex_<service>_bucket_store_series_data_size_touched_bytes` => `cortex_bucket_store_series_data_size_touched_bytes{component="<service>"}`
  * `cortex_<service>_bucket_store_series_data_size_fetched_bytes` => `cortex_bucket_store_series_data_size_fetched_bytes{component="<service>"}`
  * `cortex_<service>_bucket_store_series_blocks_queried` => `cortex_bucket_store_series_blocks_queried{component="<service>"}`
  * `cortex_<service>_bucket_store_series_get_all_duration_seconds` => `cortex_bucket_store_series_get_all_duration_seconds{component="<service>"}`
  * `cortex_<service>_bucket_store_series_merge_duration_seconds` => `cortex_bucket_store_series_merge_duration_seconds{component="<service>"}`
  * `cortex_<service>_bucket_store_series_refetches_total` => `cortex_bucket_store_series_refetches_total{component="<service>"}`
  * `cortex_<service>_bucket_store_series_result_series` => `cortex_bucket_store_series_result_series{component="<service>"}`
  * `cortex_<service>_bucket_store_cached_postings_compressions_total` => `cortex_bucket_store_cached_postings_compressions_total{component="<service>"}`
  * `cortex_<service>_bucket_store_cached_postings_compression_errors_total` => `cortex_bucket_store_cached_postings_compression_errors_total{component="<service>"}`
  * `cortex_<service>_bucket_store_cached_postings_compression_time_seconds` => `cortex_bucket_store_cached_postings_compression_time_seconds{component="<service>"}`
  * `cortex_<service>_bucket_store_cached_postings_original_size_bytes_total` => `cortex_bucket_store_cached_postings_original_size_bytes_total{component="<service>"}`
  * `cortex_<service>_bucket_store_cached_postings_compressed_size_bytes_total` => `cortex_bucket_store_cached_postings_compressed_size_bytes_total{component="<service>"}`
  * `cortex_<service>_blocks_sync_seconds` => `cortex_bucket_stores_blocks_sync_seconds{component="<service>"}`
  * `cortex_<service>_blocks_last_successful_sync_timestamp_seconds` => `cortex_bucket_stores_blocks_last_successful_sync_timestamp_seconds{component="<service>"}`
* [CHANGE] Available command-line flags are printed to stdout, and only when requested via `-help`. Using invalid flag no longer causes printing of all available flags. #2691
* [CHANGE] Experimental Memberlist ring: randomize gossip node names to avoid conflicts when running multiple clients on the same host, or reusing host names (eg. pods in statefulset). Node name randomization can be disabled by using `-memberlist.randomize-node-name=false`. #2715
* [CHANGE] Memberlist KV client is no longer considered experimental. #2725
* [CHANGE] Experimental Delete Series: Make delete request cancellation duration configurable. #2760
* [CHANGE] Removed `-store.fullsize-chunks` option which was undocumented and unused (it broke ingester hand-overs). #2656
* [CHANGE] Query with no metric name that has previously resulted in HTTP status code 500 now returns status code 422 instead. #2571
* [FEATURE] TLS config options added for GRPC clients in Querier (Query-frontend client & Ingester client), Ruler, Store Gateway, as well as HTTP client in Config store client. #2502
* [FEATURE] The flag `-frontend.max-cache-freshness` is now supported within the limits overrides, to specify per-tenant max cache freshness values. The corresponding YAML config parameter has been changed from `results_cache.max_freshness` to `limits_config.max_cache_freshness`. The legacy YAML config parameter (`results_cache.max_freshness`) will continue to be supported till Cortex release `v1.4.0`. #2609
* [FEATURE] Experimental gRPC Store: Added support to 3rd parties index and chunk stores using gRPC client/server plugin mechanism. #2220
* [FEATURE] Add `-cassandra.table-options` flag to customize table options of Cassandra when creating the index or chunk table. #2575
* [ENHANCEMENT] Propagate GOPROXY value when building `build-image`. This is to help the builders building the code in a Network where default Go proxy is not accessible (e.g. when behind some corporate VPN). #2741
* [ENHANCEMENT] Querier: Added metric `cortex_querier_request_duration_seconds` for all requests to the querier. #2708
* [ENHANCEMENT] Cortex is now built with Go 1.14. #2480 #2749 #2753
* [ENHANCEMENT] Experimental TSDB: added the following metrics to the ingester: #2580 #2583 #2589 #2654
  * `cortex_ingester_tsdb_appender_add_duration_seconds`
  * `cortex_ingester_tsdb_appender_commit_duration_seconds`
  * `cortex_ingester_tsdb_refcache_purge_duration_seconds`
  * `cortex_ingester_tsdb_compactions_total`
  * `cortex_ingester_tsdb_compaction_duration_seconds`
  * `cortex_ingester_tsdb_wal_fsync_duration_seconds`
  * `cortex_ingester_tsdb_wal_page_flushes_total`
  * `cortex_ingester_tsdb_wal_completed_pages_total`
  * `cortex_ingester_tsdb_wal_truncations_failed_total`
  * `cortex_ingester_tsdb_wal_truncations_total`
  * `cortex_ingester_tsdb_wal_writes_failed_total`
  * `cortex_ingester_tsdb_checkpoint_deletions_failed_total`
  * `cortex_ingester_tsdb_checkpoint_deletions_total`
  * `cortex_ingester_tsdb_checkpoint_creations_failed_total`
  * `cortex_ingester_tsdb_checkpoint_creations_total`
  * `cortex_ingester_tsdb_wal_truncate_duration_seconds`
  * `cortex_ingester_tsdb_head_active_appenders`
  * `cortex_ingester_tsdb_head_series_not_found_total`
  * `cortex_ingester_tsdb_head_chunks`
  * `cortex_ingester_tsdb_mmap_chunk_corruptions_total`
  * `cortex_ingester_tsdb_head_chunks_created_total`
  * `cortex_ingester_tsdb_head_chunks_removed_total`
* [ENHANCEMENT] Experimental TSDB: added metrics useful to alert on critical conditions of the blocks storage: #2573
  * `cortex_compactor_last_successful_run_timestamp_seconds`
  * `cortex_querier_blocks_last_successful_sync_timestamp_seconds` (when store-gateway is disabled)
  * `cortex_querier_blocks_last_successful_scan_timestamp_seconds` (when store-gateway is enabled)
  * `cortex_storegateway_blocks_last_successful_sync_timestamp_seconds`
* [ENHANCEMENT] Experimental TSDB: added the flag `-experimental.tsdb.wal-compression-enabled` to allow to enable TSDB WAL compression. #2585
* [ENHANCEMENT] Experimental TSDB: Querier and store-gateway components can now use so-called "caching bucket", which can currently cache fetched chunks into shared memcached server. #2572
* [ENHANCEMENT] Ruler: Automatically remove unhealthy rulers from the ring. #2587
* [ENHANCEMENT] Query-tee: added support to `/metadata`, `/alerts`, and `/rules` endpoints #2600
* [ENHANCEMENT] Query-tee: added support to query results comparison between two different backends. The comparison is disabled by default and can be enabled via `-proxy.compare-responses=true`. #2611
* [ENHANCEMENT] Query-tee: improved the query-tee to not wait all backend responses before sending back the response to the client. The query-tee now sends back to the client first successful response, while honoring the `-backend.preferred` option. #2702
* [ENHANCEMENT] Thanos and Prometheus upgraded. #2602 #2604 #2634 #2659 #2686 #2756
  * TSDB now holds less WAL files after Head Truncation.
  * TSDB now does memory-mapping of Head chunks and reduces memory usage.
* [ENHANCEMENT] Experimental TSDB: decoupled blocks deletion from blocks compaction in the compactor, so that blocks deletion is not blocked by a busy compactor. The following metrics have been added: #2623
  * `cortex_compactor_block_cleanup_started_total`
  * `cortex_compactor_block_cleanup_completed_total`
  * `cortex_compactor_block_cleanup_failed_total`
  * `cortex_compactor_block_cleanup_last_successful_run_timestamp_seconds`
* [ENHANCEMENT] Experimental TSDB: Use shared cache for metadata. This is especially useful when running multiple querier and store-gateway components to reduce number of object store API calls. #2626 #2640
* [ENHANCEMENT] Experimental TSDB: when `-querier.query-store-after` is configured and running the experimental blocks storage, the time range of the query sent to the store is now manipulated to ensure the query end time is not more recent than 'now - query-store-after'. #2642
* [ENHANCEMENT] Experimental TSDB: small performance improvement in concurrent usage of RefCache, used during samples ingestion. #2651
* [ENHANCEMENT] The following endpoints now respond appropriately to an `Accept` header with the value `application/json` #2673
  * `/distributor/all_user_stats`
  * `/distributor/ha_tracker`
  * `/ingester/ring`
  * `/store-gateway/ring`
  * `/compactor/ring`
  * `/ruler/ring`
  * `/services`
* [ENHANCEMENT] Experimental Cassandra backend: Add `-cassandra.num-connections` to allow increasing the number of TCP connections to each Cassandra server. #2666
* [ENHANCEMENT] Experimental Cassandra backend: Use separate Cassandra clients and connections for reads and writes. #2666
* [ENHANCEMENT] Experimental Cassandra backend: Add `-cassandra.reconnect-interval` to allow specifying the reconnect interval to a Cassandra server that has been marked `DOWN` by the gocql driver. Also change the default value of the reconnect interval from `60s` to `1s`. #2687
* [ENHANCEMENT] Experimental Cassandra backend: Add option `-cassandra.convict-hosts-on-failure=false` to not convict host of being down when a request fails. #2684
* [ENHANCEMENT] Experimental TSDB: Applied a jitter to the period bucket scans in order to better distribute bucket operations over the time and increase the probability of hitting the shared cache (if configured). #2693
* [ENHANCEMENT] Experimental TSDB: Series limit per user and per metric now work in TSDB blocks. #2676
* [ENHANCEMENT] Experimental Memberlist: Added ability to periodically rejoin the memberlist cluster. #2724
* [ENHANCEMENT] Experimental Delete Series: Added the following metrics for monitoring processing of delete requests: #2730
  - `cortex_purger_load_pending_requests_attempts_total`: Number of attempts that were made to load pending requests with status.
  - `cortex_purger_oldest_pending_delete_request_age_seconds`: Age of oldest pending delete request in seconds.
  - `cortex_purger_pending_delete_requests_count`: Count of requests which are in process or are ready to be processed.
* [ENHANCEMENT] Experimental TSDB: Improved compactor to hard-delete also partial blocks with an deletion mark (even if the deletion mark threshold has not been reached). #2751
* [ENHANCEMENT] Experimental TSDB: Introduced a consistency check done by the querier to ensure all expected blocks have been queried via the store-gateway. If a block is missing on a store-gateway, the querier retries fetching series from missing blocks up to 3 times. If the consistency check fails once all retries have been exhausted, the query execution fails. The following metrics have been added: #2593 #2630 #2689 #2695
  * `cortex_querier_blocks_consistency_checks_total`
  * `cortex_querier_blocks_consistency_checks_failed_total`
  * `cortex_querier_storegateway_refetches_per_query`
* [ENHANCEMENT] Delete requests can now be canceled #2555
* [ENHANCEMENT] Table manager can now provision tables for delete store #2546
* [BUGFIX] Ruler: Ensure temporary rule files with special characters are properly mapped and cleaned up. #2506
* [BUGFIX] Fixes #2411, Ensure requests are properly routed to the prometheus api embedded in the query if `-server.path-prefix` is set. #2372
* [BUGFIX] Experimental TSDB: fixed chunk data corruption when querying back series using the experimental blocks storage. #2400
* [BUGFIX] Fixed collection of tracing spans from Thanos components used internally. #2655
* [BUGFIX] Experimental TSDB: fixed memory leak in ingesters. #2586
* [BUGFIX] QueryFrontend: fixed a situation where HTTP error is ignored and an incorrect status code is set. #2590
* [BUGFIX] Ingester: Fix an ingester starting up in the JOINING state and staying there forever. #2565
* [BUGFIX] QueryFrontend: fixed a panic (`integer divide by zero`) in the query-frontend. The query-frontend now requires the `-querier.default-evaluation-interval` config to be set to the same value of the querier. #2614
* [BUGFIX] Experimental TSDB: when the querier receives a `/series` request with a time range older than the data stored in the ingester, it now ignores the requested time range and returns known series anyway instead of returning an empty response. This aligns the behaviour with the chunks storage. #2617
* [BUGFIX] Cassandra: fixed an edge case leading to an invalid CQL query when querying the index on a Cassandra store. #2639
* [BUGFIX] Ingester: increment series per metric when recovering from WAL or transfer. #2674
* [BUGFIX] Fixed `wrong number of arguments for 'mget' command` Redis error when a query has no chunks to lookup from storage. #2700 #2796
* [BUGFIX] Ingester: Automatically remove old tmp checkpoints, fixing a potential disk space leak after an ingester crashes. #2726

## 1.1.0 / 2020-05-21

This release brings the usual mix of bugfixes and improvements. The biggest change is that WAL support for chunks is now considered to be production-ready!

Please make sure to review renamed metrics, and update your dashboards and alerts accordingly.

* [CHANGE] Added v1 API routes documented in #2327. #2372
  * Added `-http.alertmanager-http-prefix` flag which allows the configuration of the path where the Alertmanager API and UI can be reached. The default is set to `/alertmanager`.
  * Added `-http.prometheus-http-prefix` flag which allows the configuration of the path where the Prometheus API and UI can be reached. The default is set to `/prometheus`.
  * Updated the index hosted at the root prefix to point to the updated routes.
  * Legacy routes hardcoded with the `/api/prom` prefix now respect the `-http.prefix` flag.
* [CHANGE] The metrics `cortex_distributor_ingester_appends_total` and `distributor_ingester_append_failures_total` now include a `type` label to differentiate between `samples` and `metadata`. #2336
* [CHANGE] The metrics for number of chunks and bytes flushed to the chunk store are renamed. Note that previous metrics were counted pre-deduplication, while new metrics are counted after deduplication. #2463
  * `cortex_ingester_chunks_stored_total` > `cortex_chunk_store_stored_chunks_total`
  * `cortex_ingester_chunk_stored_bytes_total` > `cortex_chunk_store_stored_chunk_bytes_total`
* [CHANGE] Experimental TSDB: renamed blocks meta fetcher metrics: #2375
  * `cortex_querier_bucket_store_blocks_meta_syncs_total` > `cortex_querier_blocks_meta_syncs_total`
  * `cortex_querier_bucket_store_blocks_meta_sync_failures_total` > `cortex_querier_blocks_meta_sync_failures_total`
  * `cortex_querier_bucket_store_blocks_meta_sync_duration_seconds` > `cortex_querier_blocks_meta_sync_duration_seconds`
  * `cortex_querier_bucket_store_blocks_meta_sync_consistency_delay_seconds` > `cortex_querier_blocks_meta_sync_consistency_delay_seconds`
* [CHANGE] Experimental TSDB: Modified default values for `compactor.deletion-delay` option from 48h to 12h and `-experimental.tsdb.bucket-store.ignore-deletion-marks-delay` from 24h to 6h. #2414
* [CHANGE] WAL: Default value of `-ingester.checkpoint-enabled` changed to `true`. #2416
* [CHANGE] `trace_id` field in log files has been renamed to `traceID`. #2518
* [CHANGE] Slow query log has a different output now. Previously used `url` field has been replaced with `host` and `path`, and query parameters are logged as individual log fields with `qs_` prefix. #2520
* [CHANGE] WAL: WAL and checkpoint compression is now disabled. #2436
* [CHANGE] Update in dependency `go-kit/kit` from `v0.9.0` to `v0.10.0`. HTML escaping disabled in JSON Logger. #2535
* [CHANGE] Experimental TSDB: Removed `cortex_<service>_` prefix from Thanos objstore metrics and added `component` label to distinguish which Cortex component is doing API calls to the object storage when running in single-binary mode: #2568
  - `cortex_<service>_thanos_objstore_bucket_operations_total` renamed to `thanos_objstore_bucket_operations_total{component="<name>"}`
  - `cortex_<service>_thanos_objstore_bucket_operation_failures_total` renamed to `thanos_objstore_bucket_operation_failures_total{component="<name>"}`
  - `cortex_<service>_thanos_objstore_bucket_operation_duration_seconds` renamed to `thanos_objstore_bucket_operation_duration_seconds{component="<name>"}`
  - `cortex_<service>_thanos_objstore_bucket_last_successful_upload_time` renamed to `thanos_objstore_bucket_last_successful_upload_time{component="<name>"}`
* [CHANGE] FIFO cache: The `-<prefix>.fifocache.size` CLI flag has been renamed to `-<prefix>.fifocache.max-size-items` as well as its YAML config option `size` renamed to `max_size_items`. #2319
* [FEATURE] Ruler: The `-ruler.evaluation-delay` flag was added to allow users to configure a default evaluation delay for all rules in cortex. The default value is 0 which is the current behavior. #2423
* [FEATURE] Experimental: Added a new object storage client for OpenStack Swift. #2440
* [FEATURE] TLS config options added to the Server. #2535
* [FEATURE] Experimental: Added support for `/api/v1/metadata` Prometheus-based endpoint. #2549
* [FEATURE] Add ability to limit concurrent queries to Cassandra with `-cassandra.query-concurrency` flag. #2562
* [FEATURE] Experimental TSDB: Introduced store-gateway service used by the experimental blocks storage to load and query blocks. The store-gateway optionally supports blocks sharding and replication via a dedicated hash ring, configurable via `-experimental.store-gateway.sharding-enabled` and `-experimental.store-gateway.sharding-ring.*` flags. The following metrics have been added: #2433 #2458 #2469 #2523
  * `cortex_querier_storegateway_instances_hit_per_query`
* [ENHANCEMENT] Experimental TSDB: sample ingestion errors are now reported via existing `cortex_discarded_samples_total` metric. #2370
* [ENHANCEMENT] Failures on samples at distributors and ingesters return the first validation error as opposed to the last. #2383
* [ENHANCEMENT] Experimental TSDB: Added `cortex_querier_blocks_meta_synced`, which reflects current state of synced blocks over all tenants. #2392
* [ENHANCEMENT] Added `cortex_distributor_latest_seen_sample_timestamp_seconds` metric to see how far behind Prometheus servers are in sending data. #2371
* [ENHANCEMENT] FIFO cache to support eviction based on memory usage. Added `-<prefix>.fifocache.max-size-bytes` CLI flag and YAML config option `max_size_bytes` to specify memory limit of the cache. #2319, #2527
* [ENHANCEMENT] Added `-querier.worker-match-max-concurrent`. Force worker concurrency to match the `-querier.max-concurrent` option.  Overrides `-querier.worker-parallelism`.  #2456
* [ENHANCEMENT] Added the following metrics for monitoring delete requests: #2445
  - `cortex_purger_delete_requests_received_total`: Number of delete requests received per user.
  - `cortex_purger_delete_requests_processed_total`: Number of delete requests processed per user.
  - `cortex_purger_delete_requests_chunks_selected_total`: Number of chunks selected while building delete plans per user.
  - `cortex_purger_delete_requests_processing_failures_total`: Number of delete requests processing failures per user.
* [ENHANCEMENT] Single Binary: Added query-frontend to the single binary.  Single binary users will now benefit from various query-frontend features.  Primarily: sharding, parallelization, load shedding, additional caching (if configured), and query retries. #2437
* [ENHANCEMENT] Allow 1w (where w denotes week) and 1y (where y denotes year) when setting `-store.cache-lookups-older-than` and `-store.max-look-back-period`. #2454
* [ENHANCEMENT] Optimize index queries for matchers using "a|b|c"-type regex. #2446 #2475
* [ENHANCEMENT] Added per tenant metrics for queries and chunks and bytes read from chunk store: #2463
  * `cortex_chunk_store_fetched_chunks_total` and `cortex_chunk_store_fetched_chunk_bytes_total`
  * `cortex_query_frontend_queries_total` (per tenant queries counted by the frontend)
* [ENHANCEMENT] WAL: New metrics `cortex_ingester_wal_logged_bytes_total` and `cortex_ingester_checkpoint_logged_bytes_total` added to track total bytes logged to disk for WAL and checkpoints. #2497
* [ENHANCEMENT] Add de-duplicated chunks counter `cortex_chunk_store_deduped_chunks_total` which counts every chunk not sent to the store because it was already sent by another replica. #2485
* [ENHANCEMENT] Query-frontend now also logs the POST data of long queries. #2481
* [ENHANCEMENT] WAL: Ingester WAL records now have type header and the custom WAL records have been replaced by Prometheus TSDB's WAL records. Old records will not be supported from 1.3 onwards. Note: once this is deployed, you cannot downgrade without data loss. #2436
* [ENHANCEMENT] Redis Cache: Added `idle_timeout`, `wait_on_pool_exhaustion` and `max_conn_lifetime` options to redis cache configuration. #2550
* [ENHANCEMENT] WAL: the experimental tag has been removed on the WAL in ingesters. #2560
* [ENHANCEMENT] Use newer AWS API for paginated queries - removes 'Deprecated' message from logfiles. #2452
* [ENHANCEMENT] Experimental memberlist: Add retry with backoff on memberlist join other members. #2705
* [ENHANCEMENT] Experimental TSDB: when the store-gateway sharding is enabled, unhealthy store-gateway instances are automatically removed from the ring after 10 consecutive `-experimental.store-gateway.sharding-ring.heartbeat-timeout` periods. #2526
* [BUGFIX] Ruler: Ensure temporary rule files with special characters are properly mapped and cleaned up. #2506
* [BUGFIX] Ensure requests are properly routed to the prometheus api embedded in the query if `-server.path-prefix` is set. Fixes #2411. #2372
* [BUGFIX] Experimental TSDB: Fixed chunk data corruption when querying back series using the experimental blocks storage. #2400
* [BUGFIX] Cassandra Storage: Fix endpoint TLS host verification. #2109
* [BUGFIX] Experimental TSDB: Fixed response status code from `422` to `500` when an error occurs while iterating chunks with the experimental blocks storage. #2402
* [BUGFIX] Ring: Fixed a situation where upgrading from pre-1.0 cortex with a rolling strategy caused new 1.0 ingesters to lose their zone value in the ring until manually forced to re-register. #2404
* [BUGFIX] Distributor: `/all_user_stats` now show API and Rule Ingest Rate correctly. #2457
* [BUGFIX] Fixed `version`, `revision` and `branch` labels exported by the `cortex_build_info` metric. #2468
* [BUGFIX] QueryFrontend: fixed a situation where span context missed when downstream_url is used. #2539
* [BUGFIX] Querier: Fixed a situation where querier would crash because of an unresponsive frontend instance. #2569

## 1.0.1 / 2020-04-23

* [BUGFIX] Fix gaps when querying ingesters with replication factor = 3 and 2 ingesters in the cluster. #2503

## 1.0.0 / 2020-04-02

This is the first major release of Cortex. We made a lot of **breaking changes** in this release which have been detailed below. Please also see the stability guarantees we provide as part of a major release: https://cortexmetrics.io/docs/configuration/v1guarantees/

* [CHANGE] Remove the following deprecated flags: #2339
  - `-metrics.error-rate-query` (use `-metrics.write-throttle-query` instead).
  - `-store.cardinality-cache-size` (use `-store.index-cache-read.enable-fifocache` and `-store.index-cache-read.fifocache.size` instead).
  - `-store.cardinality-cache-validity` (use `-store.index-cache-read.enable-fifocache` and `-store.index-cache-read.fifocache.duration` instead).
  - `-distributor.limiter-reload-period` (flag unused)
  - `-ingester.claim-on-rollout` (flag unused)
  - `-ingester.normalise-tokens` (flag unused)
* [CHANGE] Renamed YAML file options to be more consistent. See [full config file changes below](#config-file-breaking-changes). #2273
* [CHANGE] AWS based autoscaling has been removed. You can only use metrics based autoscaling now. `-applicationautoscaling.url` has been removed. See https://cortexmetrics.io/docs/production/aws/#dynamodb-capacity-provisioning on how to migrate. #2328
* [CHANGE] Renamed the `memcache.write-back-goroutines` and `memcache.write-back-buffer` flags to `background.write-back-concurrency` and `background.write-back-buffer`. This affects the following flags: #2241
  - `-frontend.memcache.write-back-buffer` --> `-frontend.background.write-back-buffer`
  - `-frontend.memcache.write-back-goroutines` --> `-frontend.background.write-back-concurrency`
  - `-store.index-cache-read.memcache.write-back-buffer` --> `-store.index-cache-read.background.write-back-buffer`
  - `-store.index-cache-read.memcache.write-back-goroutines` --> `-store.index-cache-read.background.write-back-concurrency`
  - `-store.index-cache-write.memcache.write-back-buffer` --> `-store.index-cache-write.background.write-back-buffer`
  - `-store.index-cache-write.memcache.write-back-goroutines` --> `-store.index-cache-write.background.write-back-concurrency`
  - `-memcache.write-back-buffer` --> `-store.chunks-cache.background.write-back-buffer`. Note the next change log for the difference.
  - `-memcache.write-back-goroutines` --> `-store.chunks-cache.background.write-back-concurrency`. Note the next change log for the difference.

* [CHANGE] Renamed the chunk cache flags to have `store.chunks-cache.` as prefix. This means the following flags have been changed: #2241
  - `-cache.enable-fifocache` --> `-store.chunks-cache.cache.enable-fifocache`
  - `-default-validity` --> `-store.chunks-cache.default-validity`
  - `-fifocache.duration` --> `-store.chunks-cache.fifocache.duration`
  - `-fifocache.size` --> `-store.chunks-cache.fifocache.size`
  - `-memcache.write-back-buffer` --> `-store.chunks-cache.background.write-back-buffer`. Note the previous change log for the difference.
  - `-memcache.write-back-goroutines` --> `-store.chunks-cache.background.write-back-concurrency`. Note the previous change log for the difference.
  - `-memcached.batchsize` --> `-store.chunks-cache.memcached.batchsize`
  - `-memcached.consistent-hash` --> `-store.chunks-cache.memcached.consistent-hash`
  - `-memcached.expiration` --> `-store.chunks-cache.memcached.expiration`
  - `-memcached.hostname` --> `-store.chunks-cache.memcached.hostname`
  - `-memcached.max-idle-conns` --> `-store.chunks-cache.memcached.max-idle-conns`
  - `-memcached.parallelism` --> `-store.chunks-cache.memcached.parallelism`
  - `-memcached.service` --> `-store.chunks-cache.memcached.service`
  - `-memcached.timeout` --> `-store.chunks-cache.memcached.timeout`
  - `-memcached.update-interval` --> `-store.chunks-cache.memcached.update-interval`
  - `-redis.enable-tls` --> `-store.chunks-cache.redis.enable-tls`
  - `-redis.endpoint` --> `-store.chunks-cache.redis.endpoint`
  - `-redis.expiration` --> `-store.chunks-cache.redis.expiration`
  - `-redis.max-active-conns` --> `-store.chunks-cache.redis.max-active-conns`
  - `-redis.max-idle-conns` --> `-store.chunks-cache.redis.max-idle-conns`
  - `-redis.password` --> `-store.chunks-cache.redis.password`
  - `-redis.timeout` --> `-store.chunks-cache.redis.timeout`
* [CHANGE] Rename the `-store.chunk-cache-stubs` to `-store.chunks-cache.cache-stubs` to be more inline with above. #2241
* [CHANGE] Change prefix of flags `-dynamodb.periodic-table.*` to `-table-manager.index-table.*`. #2359
* [CHANGE] Change prefix of flags `-dynamodb.chunk-table.*` to `-table-manager.chunk-table.*`. #2359
* [CHANGE] Change the following flags: #2359
  - `-dynamodb.poll-interval` --> `-table-manager.poll-interval`
  - `-dynamodb.periodic-table.grace-period` --> `-table-manager.periodic-table.grace-period`
* [CHANGE] Renamed the following flags: #2273
  - `-dynamodb.chunk.gang.size` --> `-dynamodb.chunk-gang-size`
  - `-dynamodb.chunk.get.max.parallelism` --> `-dynamodb.chunk-get-max-parallelism`
* [CHANGE] Don't support mixed time units anymore for duration. For example, 168h5m0s doesn't work anymore, please use just one unit (s|m|h|d|w|y). #2252
* [CHANGE] Utilize separate protos for rule state and storage. Experimental ruler API will not be functional until the rollout is complete. #2226
* [CHANGE] Frontend worker in querier now starts after all Querier module dependencies are started. This fixes issue where frontend worker started to send queries to querier before it was ready to serve them (mostly visible when using experimental blocks storage). #2246
* [CHANGE] Lifecycler component now enters Failed state on errors, and doesn't exit the process. (Important if you're vendoring Cortex and use Lifecycler) #2251
* [CHANGE] `/ready` handler now returns 200 instead of 204. #2330
* [CHANGE] Better defaults for the following options: #2344
  - `-<prefix>.consul.consistent-reads`: Old default: `true`, new default: `false`. This reduces the load on Consul.
  - `-<prefix>.consul.watch-rate-limit`: Old default: 0, new default: 1. This rate limits the reads to 1 per second. Which is good enough for ring watches.
  - `-distributor.health-check-ingesters`: Old default: `false`, new default: `true`.
  - `-ingester.max-stale-chunk-idle`: Old default: 0, new default: 2m. This lets us expire series that we know are stale early.
  - `-ingester.spread-flushes`: Old default: false, new default: true. This allows to better de-duplicate data and use less space.
  - `-ingester.chunk-age-jitter`: Old default: 20mins, new default: 0. This is to enable the `-ingester.spread-flushes` to true.
  - `-<prefix>.memcached.batchsize`: Old default: 0, new default: 1024. This allows batching of requests and keeps the concurrent requests low.
  - `-<prefix>.memcached.consistent-hash`: Old default: false, new default: true. This allows for better cache hits when the memcaches are scaled up and down.
  - `-querier.batch-iterators`: Old default: false, new default: true.
  - `-querier.ingester-streaming`: Old default: false, new default: true.
* [CHANGE] Experimental TSDB: Added `-experimental.tsdb.bucket-store.postings-cache-compression-enabled` to enable postings compression when storing to cache. #2335
* [CHANGE] Experimental TSDB: Added `-compactor.deletion-delay`, which is time before a block marked for deletion is deleted from bucket. If not 0, blocks will be marked for deletion and compactor component will delete blocks marked for deletion from the bucket. If delete-delay is 0, blocks will be deleted straight away. Note that deleting blocks immediately can cause query failures, if store gateway / querier still has the block loaded, or compactor is ignoring the deletion because it's compacting the block at the same time. Default value is 48h. #2335
* [CHANGE] Experimental TSDB: Added `-experimental.tsdb.bucket-store.index-cache.postings-compression-enabled`, to set duration after which the blocks marked for deletion will be filtered out while fetching blocks used for querying. This option allows querier to ignore blocks that are marked for deletion with some delay. This ensures store can still serve blocks that are meant to be deleted but do not have a replacement yet. Default is 24h, half of the default value for `-compactor.deletion-delay`. #2335
* [CHANGE] Experimental TSDB: Added `-experimental.tsdb.bucket-store.index-cache.memcached.max-item-size` to control maximum size of item that is stored to memcached. Defaults to 1 MiB. #2335
* [FEATURE] Added experimental storage API to the ruler service that is enabled when the `-experimental.ruler.enable-api` is set to true #2269
  * `-ruler.storage.type` flag now allows `s3`,`gcs`, and `azure` values
  * `-ruler.storage.(s3|gcs|azure)` flags exist to allow the configuration of object clients set for rule storage
* [CHANGE] Renamed table manager metrics. #2307 #2359
  * `cortex_dynamo_sync_tables_seconds` -> `cortex_table_manager_sync_duration_seconds`
  * `cortex_dynamo_table_capacity_units` -> `cortex_table_capacity_units`
* [FEATURE] Flusher target to flush the WAL. #2075
  * `-flusher.wal-dir` for the WAL directory to recover from.
  * `-flusher.concurrent-flushes` for number of concurrent flushes.
  * `-flusher.flush-op-timeout` is duration after which a flush should timeout.
* [FEATURE] Ingesters can now have an optional availability zone set, to ensure metric replication is distributed across zones. This is set via the `-ingester.availability-zone` flag or the `availability_zone` field in the config file. #2317
* [ENHANCEMENT] Better reuse of connections to DynamoDB and S3. #2268
* [ENHANCEMENT] Reduce number of goroutines used while executing a single index query. #2280
* [ENHANCEMENT] Experimental TSDB: Add support for local `filesystem` backend. #2245
* [ENHANCEMENT] Experimental TSDB: Added memcached support for the TSDB index cache. #2290
* [ENHANCEMENT] Experimental TSDB: Removed gRPC server to communicate between querier and BucketStore. #2324
* [ENHANCEMENT] Allow 1w (where w denotes week) and 1y (where y denotes year) when setting table period and retention. #2252
* [ENHANCEMENT] Added FIFO cache metrics for current number of entries and memory usage. #2270
* [ENHANCEMENT] Output all config fields to /config API, including those with empty value. #2209
* [ENHANCEMENT] Add "missing_metric_name" and "metric_name_invalid" reasons to cortex_discarded_samples_total metric. #2346
* [ENHANCEMENT] Experimental TSDB: sample ingestion errors are now reported via existing `cortex_discarded_samples_total` metric. #2370
* [BUGFIX] Ensure user state metrics are updated if a transfer fails. #2338
* [BUGFIX] Fixed etcd client keepalive settings. #2278
* [BUGFIX] Register the metrics of the WAL. #2295
* [BUXFIX] Experimental TSDB: fixed error handling when ingesting out of bound samples. #2342

### Known issues

- This experimental blocks storage in Cortex `1.0.0` has a bug which may lead to the error `cannot iterate chunk for series` when running queries. This bug has been fixed in #2400. If you're running the experimental blocks storage, please build Cortex from `master`.

### Config file breaking changes

In this section you can find a config file diff showing the breaking changes introduced in Cortex. You can also find the [full configuration file reference doc](https://cortexmetrics.io/docs/configuration/configuration-file/) in the website.

```diff
### ingester_config

 # Period with which to attempt to flush chunks.
 # CLI flag: -ingester.flush-period
-[flushcheckperiod: <duration> | default = 1m0s]
+[flush_period: <duration> | default = 1m0s]

 # Period chunks will remain in memory after flushing.
 # CLI flag: -ingester.retain-period
-[retainperiod: <duration> | default = 5m0s]
+[retain_period: <duration> | default = 5m0s]

 # Maximum chunk idle time before flushing.
 # CLI flag: -ingester.max-chunk-idle
-[maxchunkidle: <duration> | default = 5m0s]
+[max_chunk_idle_time: <duration> | default = 5m0s]

 # Maximum chunk idle time for chunks terminating in stale markers before
 # flushing. 0 disables it and a stale series is not flushed until the
 # max-chunk-idle timeout is reached.
 # CLI flag: -ingester.max-stale-chunk-idle
-[maxstalechunkidle: <duration> | default = 0s]
+[max_stale_chunk_idle_time: <duration> | default = 2m0s]

 # Timeout for individual flush operations.
 # CLI flag: -ingester.flush-op-timeout
-[flushoptimeout: <duration> | default = 1m0s]
+[flush_op_timeout: <duration> | default = 1m0s]

 # Maximum chunk age before flushing.
 # CLI flag: -ingester.max-chunk-age
-[maxchunkage: <duration> | default = 12h0m0s]
+[max_chunk_age: <duration> | default = 12h0m0s]

-# Range of time to subtract from MaxChunkAge to spread out flushes
+# Range of time to subtract from -ingester.max-chunk-age to spread out flushes
 # CLI flag: -ingester.chunk-age-jitter
-[chunkagejitter: <duration> | default = 20m0s]
+[chunk_age_jitter: <duration> | default = 0]

 # Number of concurrent goroutines flushing to dynamodb.
 # CLI flag: -ingester.concurrent-flushes
-[concurrentflushes: <int> | default = 50]
+[concurrent_flushes: <int> | default = 50]

-# If true, spread series flushes across the whole period of MaxChunkAge
+# If true, spread series flushes across the whole period of
+# -ingester.max-chunk-age.
 # CLI flag: -ingester.spread-flushes
-[spreadflushes: <boolean> | default = false]
+[spread_flushes: <boolean> | default = true]

 # Period with which to update the per-user ingestion rates.
 # CLI flag: -ingester.rate-update-period
-[rateupdateperiod: <duration> | default = 15s]
+[rate_update_period: <duration> | default = 15s]


### querier_config

 # The maximum number of concurrent queries.
 # CLI flag: -querier.max-concurrent
-[maxconcurrent: <int> | default = 20]
+[max_concurrent: <int> | default = 20]

 # Use batch iterators to execute query, as opposed to fully materialising the
 # series in memory.  Takes precedent over the -querier.iterators flag.
 # CLI flag: -querier.batch-iterators
-[batchiterators: <boolean> | default = false]
+[batch_iterators: <boolean> | default = true]

 # Use streaming RPCs to query ingester.
 # CLI flag: -querier.ingester-streaming
-[ingesterstreaming: <boolean> | default = false]
+[ingester_streaming: <boolean> | default = true]

 # Maximum number of samples a single query can load into memory.
 # CLI flag: -querier.max-samples
-[maxsamples: <int> | default = 50000000]
+[max_samples: <int> | default = 50000000]

 # The default evaluation interval or step size for subqueries.
 # CLI flag: -querier.default-evaluation-interval
-[defaultevaluationinterval: <duration> | default = 1m0s]
+[default_evaluation_interval: <duration> | default = 1m0s]

### query_frontend_config

 # URL of downstream Prometheus.
 # CLI flag: -frontend.downstream-url
-[downstream: <string> | default = ""]
+[downstream_url: <string> | default = ""]


### ruler_config

 # URL of alerts return path.
 # CLI flag: -ruler.external.url
-[externalurl: <url> | default = ]
+[external_url: <url> | default = ]

 # How frequently to evaluate rules
 # CLI flag: -ruler.evaluation-interval
-[evaluationinterval: <duration> | default = 1m0s]
+[evaluation_interval: <duration> | default = 1m0s]

 # How frequently to poll for rule changes
 # CLI flag: -ruler.poll-interval
-[pollinterval: <duration> | default = 1m0s]
+[poll_interval: <duration> | default = 1m0s]

-storeconfig:
+storage:

 # file path to store temporary rule files for the prometheus rule managers
 # CLI flag: -ruler.rule-path
-[rulepath: <string> | default = "/rules"]
+[rule_path: <string> | default = "/rules"]

 # URL of the Alertmanager to send notifications to.
 # CLI flag: -ruler.alertmanager-url
-[alertmanagerurl: <url> | default = ]
+[alertmanager_url: <url> | default = ]

 # Use DNS SRV records to discover alertmanager hosts.
 # CLI flag: -ruler.alertmanager-discovery
-[alertmanagerdiscovery: <boolean> | default = false]
+[enable_alertmanager_discovery: <boolean> | default = false]

 # How long to wait between refreshing alertmanager hosts.
 # CLI flag: -ruler.alertmanager-refresh-interval
-[alertmanagerrefreshinterval: <duration> | default = 1m0s]
+[alertmanager_refresh_interval: <duration> | default = 1m0s]

 # If enabled requests to alertmanager will utilize the V2 API.
 # CLI flag: -ruler.alertmanager-use-v2
-[alertmanangerenablev2api: <boolean> | default = false]
+[enable_alertmanager_v2: <boolean> | default = false]

 # Capacity of the queue for notifications to be sent to the Alertmanager.
 # CLI flag: -ruler.notification-queue-capacity
-[notificationqueuecapacity: <int> | default = 10000]
+[notification_queue_capacity: <int> | default = 10000]

 # HTTP timeout duration when sending notifications to the Alertmanager.
 # CLI flag: -ruler.notification-timeout
-[notificationtimeout: <duration> | default = 10s]
+[notification_timeout: <duration> | default = 10s]

 # Distribute rule evaluation using ring backend
 # CLI flag: -ruler.enable-sharding
-[enablesharding: <boolean> | default = false]
+[enable_sharding: <boolean> | default = false]

 # Time to spend searching for a pending ruler when shutting down.
 # CLI flag: -ruler.search-pending-for
-[searchpendingfor: <duration> | default = 5m0s]
+[search_pending_for: <duration> | default = 5m0s]

 # Period with which to attempt to flush rule groups.
 # CLI flag: -ruler.flush-period
-[flushcheckperiod: <duration> | default = 1m0s]
+[flush_period: <duration> | default = 1m0s]

### alertmanager_config

 # Base path for data storage.
 # CLI flag: -alertmanager.storage.path
-[datadir: <string> | default = "data/"]
+[data_dir: <string> | default = "data/"]

 # will be used to prefix all HTTP endpoints served by Alertmanager. If omitted,
 # relevant URL components will be derived automatically.
 # CLI flag: -alertmanager.web.external-url
-[externalurl: <url> | default = ]
+[external_url: <url> | default = ]

 # How frequently to poll Cortex configs
 # CLI flag: -alertmanager.configs.poll-interval
-[pollinterval: <duration> | default = 15s]
+[poll_interval: <duration> | default = 15s]

 # Listen address for cluster.
 # CLI flag: -cluster.listen-address
-[clusterbindaddr: <string> | default = "0.0.0.0:9094"]
+[cluster_bind_address: <string> | default = "0.0.0.0:9094"]

 # Explicit address to advertise in cluster.
 # CLI flag: -cluster.advertise-address
-[clusteradvertiseaddr: <string> | default = ""]
+[cluster_advertise_address: <string> | default = ""]

 # Time to wait between peers to send notifications.
 # CLI flag: -cluster.peer-timeout
-[peertimeout: <duration> | default = 15s]
+[peer_timeout: <duration> | default = 15s]

 # Filename of fallback config to use if none specified for instance.
 # CLI flag: -alertmanager.configs.fallback
-[fallbackconfigfile: <string> | default = ""]
+[fallback_config_file: <string> | default = ""]

 # Root of URL to generate if config is http://internal.monitor
 # CLI flag: -alertmanager.configs.auto-webhook-root
-[autowebhookroot: <string> | default = ""]
+[auto_webhook_root: <string> | default = ""]

### table_manager_config

-store:
+storage:

-# How frequently to poll DynamoDB to learn our capacity.
-# CLI flag: -dynamodb.poll-interval
-[dynamodb_poll_interval: <duration> | default = 2m0s]
+# How frequently to poll backend to learn our capacity.
+# CLI flag: -table-manager.poll-interval
+[poll_interval: <duration> | default = 2m0s]

-# DynamoDB periodic tables grace period (duration which table will be
-# created/deleted before/after it's needed).
-# CLI flag: -dynamodb.periodic-table.grace-period
+# Periodic tables grace period (duration which table will be created/deleted
+# before/after it's needed).
+# CLI flag: -table-manager.periodic-table.grace-period
 [creation_grace_period: <duration> | default = 10m0s]

 index_tables_provisioning:
   # Enables on demand throughput provisioning for the storage provider (if
-  # supported). Applies only to tables which are not autoscaled
-  # CLI flag: -dynamodb.periodic-table.enable-ondemand-throughput-mode
-  [provisioned_throughput_on_demand_mode: <boolean> | default = false]
+  # supported). Applies only to tables which are not autoscaled. Supported by
+  # DynamoDB
+  # CLI flag: -table-manager.index-table.enable-ondemand-throughput-mode
+  [enable_ondemand_throughput_mode: <boolean> | default = false]


   # Enables on demand throughput provisioning for the storage provider (if
-  # supported). Applies only to tables which are not autoscaled
-  # CLI flag: -dynamodb.periodic-table.inactive-enable-ondemand-throughput-mode
-  [inactive_throughput_on_demand_mode: <boolean> | default = false]
+  # supported). Applies only to tables which are not autoscaled. Supported by
+  # DynamoDB
+  # CLI flag: -table-manager.index-table.inactive-enable-ondemand-throughput-mode
+  [enable_inactive_throughput_on_demand_mode: <boolean> | default = false]


 chunk_tables_provisioning:
   # Enables on demand throughput provisioning for the storage provider (if
-  # supported). Applies only to tables which are not autoscaled
-  # CLI flag: -dynamodb.chunk-table.enable-ondemand-throughput-mode
-  [provisioned_throughput_on_demand_mode: <boolean> | default = false]
+  # supported). Applies only to tables which are not autoscaled. Supported by
+  # DynamoDB
+  # CLI flag: -table-manager.chunk-table.enable-ondemand-throughput-mode
+  [enable_ondemand_throughput_mode: <boolean> | default = false]

### storage_config

 aws:
-  dynamodbconfig:
+  dynamodb:
     # DynamoDB endpoint URL with escaped Key and Secret encoded. If only region
     # is specified as a host, proper endpoint will be deduced. Use
     # inmemory:///<table-name> to use a mock in-memory implementation.
     # CLI flag: -dynamodb.url
-    [dynamodb: <url> | default = ]
+    [dynamodb_url: <url> | default = ]

     # DynamoDB table management requests per second limit.
     # CLI flag: -dynamodb.api-limit
-    [apilimit: <float> | default = 2]
+    [api_limit: <float> | default = 2]

     # DynamoDB rate cap to back off when throttled.
     # CLI flag: -dynamodb.throttle-limit
-    [throttlelimit: <float> | default = 10]
+    [throttle_limit: <float> | default = 10]
-
-    # ApplicationAutoscaling endpoint URL with escaped Key and Secret encoded.
-    # CLI flag: -applicationautoscaling.url
-    [applicationautoscaling: <url> | default = ]


       # Queue length above which we will scale up capacity
       # CLI flag: -metrics.target-queue-length
-      [targetqueuelen: <int> | default = 100000]
+      [target_queue_length: <int> | default = 100000]

       # Scale up capacity by this multiple
       # CLI flag: -metrics.scale-up-factor
-      [scaleupfactor: <float> | default = 1.3]
+      [scale_up_factor: <float> | default = 1.3]

       # Ignore throttling below this level (rate per second)
       # CLI flag: -metrics.ignore-throttle-below
-      [minthrottling: <float> | default = 1]
+      [ignore_throttle_below: <float> | default = 1]

       # query to fetch ingester queue length
       # CLI flag: -metrics.queue-length-query
-      [queuelengthquery: <string> | default = "sum(avg_over_time(cortex_ingester_flush_queue_length{job=\"cortex/ingester\"}[2m]))"]
+      [queue_length_query: <string> | default = "sum(avg_over_time(cortex_ingester_flush_queue_length{job=\"cortex/ingester\"}[2m]))"]

       # query to fetch throttle rates per table
       # CLI flag: -metrics.write-throttle-query
-      [throttlequery: <string> | default = "sum(rate(cortex_dynamo_throttled_total{operation=\"DynamoDB.BatchWriteItem\"}[1m])) by (table) > 0"]
+      [write_throttle_query: <string> | default = "sum(rate(cortex_dynamo_throttled_total{operation=\"DynamoDB.BatchWriteItem\"}[1m])) by (table) > 0"]

       # query to fetch write capacity usage per table
       # CLI flag: -metrics.usage-query
-      [usagequery: <string> | default = "sum(rate(cortex_dynamo_consumed_capacity_total{operation=\"DynamoDB.BatchWriteItem\"}[15m])) by (table) > 0"]
+      [write_usage_query: <string> | default = "sum(rate(cortex_dynamo_consumed_capacity_total{operation=\"DynamoDB.BatchWriteItem\"}[15m])) by (table) > 0"]

       # query to fetch read capacity usage per table
       # CLI flag: -metrics.read-usage-query
-      [readusagequery: <string> | default = "sum(rate(cortex_dynamo_consumed_capacity_total{operation=\"DynamoDB.QueryPages\"}[1h])) by (table) > 0"]
+      [read_usage_query: <string> | default = "sum(rate(cortex_dynamo_consumed_capacity_total{operation=\"DynamoDB.QueryPages\"}[1h])) by (table) > 0"]

       # query to fetch read errors per table
       # CLI flag: -metrics.read-error-query
-      [readerrorquery: <string> | default = "sum(increase(cortex_dynamo_failures_total{operation=\"DynamoDB.QueryPages\",error=\"ProvisionedThroughputExceededException\"}[1m])) by (table) > 0"]
+      [read_error_query: <string> | default = "sum(increase(cortex_dynamo_failures_total{operation=\"DynamoDB.QueryPages\",error=\"ProvisionedThroughputExceededException\"}[1m])) by (table) > 0"]

     # Number of chunks to group together to parallelise fetches (zero to
     # disable)
-    # CLI flag: -dynamodb.chunk.gang.size
-    [chunkgangsize: <int> | default = 10]
+    # CLI flag: -dynamodb.chunk-gang-size
+    [chunk_gang_size: <int> | default = 10]

     # Max number of chunk-get operations to start in parallel
-    # CLI flag: -dynamodb.chunk.get.max.parallelism
-    [chunkgetmaxparallelism: <int> | default = 32]
+    # CLI flag: -dynamodb.chunk.get-max-parallelism
+    [chunk_get_max_parallelism: <int> | default = 32]

     backoff_config:
       # Minimum delay when backing off.
       # CLI flag: -bigtable.backoff-min-period
-      [minbackoff: <duration> | default = 100ms]
+      [min_period: <duration> | default = 100ms]

       # Maximum delay when backing off.
       # CLI flag: -bigtable.backoff-max-period
-      [maxbackoff: <duration> | default = 10s]
+      [max_period: <duration> | default = 10s]

       # Number of times to backoff and retry before failing.
       # CLI flag: -bigtable.backoff-retries
-      [maxretries: <int> | default = 10]
+      [max_retries: <int> | default = 10]

   # If enabled, once a tables info is fetched, it is cached.
   # CLI flag: -bigtable.table-cache.enabled
-  [tablecacheenabled: <boolean> | default = true]
+  [table_cache_enabled: <boolean> | default = true]

   # Duration to cache tables before checking again.
   # CLI flag: -bigtable.table-cache.expiration
-  [tablecacheexpiration: <duration> | default = 30m0s]
+  [table_cache_expiration: <duration> | default = 30m0s]

 # Cache validity for active index entries. Should be no higher than
 # -ingester.max-chunk-idle.
 # CLI flag: -store.index-cache-validity
-[indexcachevalidity: <duration> | default = 5m0s]
+[index_cache_validity: <duration> | default = 5m0s]

### ingester_client_config

 grpc_client_config:
   backoff_config:
     # Minimum delay when backing off.
     # CLI flag: -ingester.client.backoff-min-period
-    [minbackoff: <duration> | default = 100ms]
+    [min_period: <duration> | default = 100ms]

     # Maximum delay when backing off.
     # CLI flag: -ingester.client.backoff-max-period
-    [maxbackoff: <duration> | default = 10s]
+    [max_period: <duration> | default = 10s]

     # Number of times to backoff and retry before failing.
     # CLI flag: -ingester.client.backoff-retries
-    [maxretries: <int> | default = 10]
+    [max_retries: <int> | default = 10]

### frontend_worker_config

-# Address of query frontend service.
+# Address of query frontend service, in host:port format.
 # CLI flag: -querier.frontend-address
-[address: <string> | default = ""]
+[frontend_address: <string> | default = ""]

 # How often to query DNS.
 # CLI flag: -querier.dns-lookup-period
-[dnslookupduration: <duration> | default = 10s]
+[dns_lookup_duration: <duration> | default = 10s]

 grpc_client_config:
   backoff_config:
     # Minimum delay when backing off.
     # CLI flag: -querier.frontend-client.backoff-min-period
-    [minbackoff: <duration> | default = 100ms]
+    [min_period: <duration> | default = 100ms]

     # Maximum delay when backing off.
     # CLI flag: -querier.frontend-client.backoff-max-period
-    [maxbackoff: <duration> | default = 10s]
+    [max_period: <duration> | default = 10s]

     # Number of times to backoff and retry before failing.
     # CLI flag: -querier.frontend-client.backoff-retries
-    [maxretries: <int> | default = 10]
+    [max_retries: <int> | default = 10]

### consul_config

 # ACL Token used to interact with Consul.
-# CLI flag: -<prefix>.consul.acltoken
-[acltoken: <string> | default = ""]
+# CLI flag: -<prefix>.consul.acl-token
+[acl_token: <string> | default = ""]

 # HTTP timeout when talking to Consul
 # CLI flag: -<prefix>.consul.client-timeout
-[httpclienttimeout: <duration> | default = 20s]
+[http_client_timeout: <duration> | default = 20s]

 # Enable consistent reads to Consul.
 # CLI flag: -<prefix>.consul.consistent-reads
-[consistentreads: <boolean> | default = true]
+[consistent_reads: <boolean> | default = false]

 # Rate limit when watching key or prefix in Consul, in requests per second. 0
 # disables the rate limit.
 # CLI flag: -<prefix>.consul.watch-rate-limit
-[watchkeyratelimit: <float> | default = 0]
+[watch_rate_limit: <float> | default = 1]

 # Burst size used in rate limit. Values less than 1 are treated as 1.
 # CLI flag: -<prefix>.consul.watch-burst-size
-[watchkeyburstsize: <int> | default = 1]
+[watch_burst_size: <int> | default = 1]


### configstore_config
 # URL of configs API server.
 # CLI flag: -<prefix>.configs.url
-[configsapiurl: <url> | default = ]
+[configs_api_url: <url> | default = ]

 # Timeout for requests to Weave Cloud configs service.
 # CLI flag: -<prefix>.configs.client-timeout
-[clienttimeout: <duration> | default = 5s]
+[client_timeout: <duration> | default = 5s]
```

## 0.7.0 / 2020-03-16

Cortex `0.7.0` is a major step forward the upcoming `1.0` release. In this release, we've got 164 contributions from 26 authors. Thanks to all contributors! ❤️

Please be aware that Cortex `0.7.0` introduces some **breaking changes**. You're encouraged to read all the `[CHANGE]` entries below before upgrading your Cortex cluster. In particular:

- Cleaned up some configuration options in preparation for the Cortex `1.0.0` release (see also the [annotated config file breaking changes](#annotated-config-file-breaking-changes) below):
  - Removed CLI flags support to configure the schema (see [how to migrate from flags to schema file](https://cortexmetrics.io/docs/configuration/schema-configuration/#migrating-from-flags-to-schema-file))
  - Renamed CLI flag `-config-yaml` to `-schema-config-file`
  - Removed CLI flag `-store.min-chunk-age` in favor of `-querier.query-store-after`. The corresponding YAML config option `ingestermaxquerylookback` has been renamed to [`query_ingesters_within`](https://cortexmetrics.io/docs/configuration/configuration-file/#querier-config)
  - Deprecated CLI flag `-frontend.cache-split-interval` in favor of `-querier.split-queries-by-interval`
  - Renamed the YAML config option `defaul_validity` to `default_validity`
  - Removed the YAML config option `config_store` (in the [`alertmanager YAML config`](https://cortexmetrics.io/docs/configuration/configuration-file/#alertmanager-config)) in favor of `store`
  - Removed the YAML config root block `configdb` in favor of [`configs`](https://cortexmetrics.io/docs/configuration/configuration-file/#configs-config). This change is also reflected in the following CLI flags renaming:
      * `-database.*` -> `-configs.database.*`
      * `-database.migrations` -> `-configs.database.migrations-dir`
  - Removed the fluentd-based billing infrastructure including the CLI flags:
      * `-distributor.enable-billing`
      * `-billing.max-buffered-events`
      * `-billing.retry-delay`
      * `-billing.ingester`
- Removed support for using denormalised tokens in the ring. Before upgrading, make sure your Cortex cluster is already running `v0.6.0` or an earlier version with `-ingester.normalise-tokens=true`

### Full changelog

* [CHANGE] Removed support for flags to configure schema. Further, the flag for specifying the config file (`-config-yaml`) has been deprecated. Please use `-schema-config-file`. See the [Schema Configuration documentation](https://cortexmetrics.io/docs/configuration/schema-configuration/) for more details on how to configure the schema using the YAML file. #2221
* [CHANGE] In the config file, the root level `config_store` config option has been moved to `alertmanager` > `store` > `configdb`. #2125
* [CHANGE] Removed unnecessary `frontend.cache-split-interval` in favor of `querier.split-queries-by-interval` both to reduce configuration complexity and guarantee alignment of these two configs. Starting from now, `-querier.cache-results` may only be enabled in conjunction with `-querier.split-queries-by-interval` (previously the cache interval default was `24h` so if you want to preserve the same behaviour you should set `-querier.split-queries-by-interval=24h`). #2040
* [CHANGE] Renamed Configs configuration options. #2187
  * configuration options
    * `-database.*` -> `-configs.database.*`
    * `-database.migrations` -> `-configs.database.migrations-dir`
  * config file
    * `configdb.uri:` -> `configs.database.uri:`
    * `configdb.migrationsdir:` -> `configs.database.migrations_dir:`
    * `configdb.passwordfile:` -> `configs.database.password_file:`
* [CHANGE] Moved `-store.min-chunk-age` to the Querier config as `-querier.query-store-after`, allowing the store to be skipped during query time if the metrics wouldn't be found. The YAML config option `ingestermaxquerylookback` has been renamed to `query_ingesters_within` to match its CLI flag. #1893
* [CHANGE] Renamed the cache configuration setting `defaul_validity` to `default_validity`. #2140
* [CHANGE] Remove fluentd-based billing infrastructure and flags such as `-distributor.enable-billing`. #1491
* [CHANGE] Removed remaining support for using denormalised tokens in the ring. If you're still running ingesters with denormalised tokens (Cortex 0.4 or earlier, with `-ingester.normalise-tokens=false`), such ingesters will now be completely invisible to distributors and need to be either switched to Cortex 0.6.0 or later, or be configured to use normalised tokens. #2034
* [CHANGE] The frontend http server will now send 502 in case of deadline exceeded and 499 if the user requested cancellation. #2156
* [CHANGE] We now enforce queries to be up to `-querier.max-query-into-future` into the future (defaults to 10m). #1929
  * `-store.min-chunk-age` has been removed
  * `-querier.query-store-after` has been added in it's place.
* [CHANGE] Removed unused `/validate_expr endpoint`. #2152
* [CHANGE] Updated Prometheus dependency to v2.16.0. This Prometheus version uses Active Query Tracker to limit concurrent queries. In order to keep `-querier.max-concurrent` working, Active Query Tracker is enabled by default, and is configured to store its data to `active-query-tracker` directory (relative to current directory when Cortex started). This can be changed by using `-querier.active-query-tracker-dir` option. Purpose of Active Query Tracker is to log queries that were running when Cortex crashes. This logging happens on next Cortex start. #2088
* [CHANGE] Default to BigChunk encoding; may result in slightly higher disk usage if many timeseries have a constant value, but should generally result in fewer, bigger chunks. #2207
* [CHANGE] WAL replays are now done while the rest of Cortex is starting, and more specifically, when HTTP server is running. This makes it possible to scrape metrics during WAL replays. Applies to both chunks and experimental blocks storage. #2222
* [CHANGE] Cortex now has `/ready` probe for all services, not just ingester and querier as before. In single-binary mode, /ready reports 204 only if all components are running properly. #2166
* [CHANGE] If you are vendoring Cortex and use its components in your project, be aware that many Cortex components no longer start automatically when they are created. You may want to review PR and attached document. #2166
* [CHANGE] Experimental TSDB: the querier in-memory index cache used by the experimental blocks storage shifted from per-tenant to per-querier. The `-experimental.tsdb.bucket-store.index-cache-size-bytes` now configures the per-querier index cache max size instead of a per-tenant cache and its default has been increased to 1GB. #2189
* [CHANGE] Experimental TSDB: TSDB head compaction interval and concurrency is now configurable (defaults to 1 min interval and 5 concurrent head compactions). New options: `-experimental.tsdb.head-compaction-interval` and `-experimental.tsdb.head-compaction-concurrency`. #2172
* [CHANGE] Experimental TSDB: switched the blocks storage index header to the binary format. This change is expected to have no visible impact, except lower startup times and memory usage in the queriers. It's possible to switch back to the old JSON format via the flag `-experimental.tsdb.bucket-store.binary-index-header-enabled=false`. #2223
* [CHANGE] Experimental Memberlist KV store can now be used in single-binary Cortex. Attempts to use it previously would fail with panic. This change also breaks existing binary protocol used to exchange gossip messages, so this version will not be able to understand gossiped Ring when used in combination with the previous version of Cortex. Easiest way to upgrade is to shutdown old Cortex installation, and restart it with new version. Incremental rollout works too, but with reduced functionality until all components run the same version. #2016
* [FEATURE] Added a read-only local alertmanager config store using files named corresponding to their tenant id. #2125
* [FEATURE] Added flag `-experimental.ruler.enable-api` to enable the ruler api which implements the Prometheus API `/api/v1/rules` and `/api/v1/alerts` endpoints under the configured `-http.prefix`. #1999
* [FEATURE] Added sharding support to compactor when using the experimental TSDB blocks storage. #2113
* [FEATURE] Added ability to override YAML config file settings using environment variables. #2147
  * `-config.expand-env`
* [FEATURE] Added flags to disable Alertmanager notifications methods. #2187
  * `-configs.notifications.disable-email`
  * `-configs.notifications.disable-webhook`
* [FEATURE] Add /config HTTP endpoint which exposes the current Cortex configuration as YAML. #2165
* [FEATURE] Allow Prometheus remote write directly to ingesters. #1491
* [FEATURE] Introduced new standalone service `query-tee` that can be used for testing purposes to send the same Prometheus query to multiple backends (ie. two Cortex clusters ingesting the same metrics) and compare the performances. #2203
* [FEATURE] Fan out parallelizable queries to backend queriers concurrently. #1878
  * `querier.parallelise-shardable-queries` (bool)
  * Requires a shard-compatible schema (v10+)
  * This causes the number of traces to increase accordingly.
  * The query-frontend now requires a schema config to determine how/when to shard queries, either from a file or from flags (i.e. by the `config-yaml` CLI flag). This is the same schema config the queriers consume. The schema is only required to use this option.
  * It's also advised to increase downstream concurrency controls as well:
    * `querier.max-outstanding-requests-per-tenant`
    * `querier.max-query-parallelism`
    * `querier.max-concurrent`
    * `server.grpc-max-concurrent-streams` (for both query-frontends and queriers)
* [FEATURE] Added user sub rings to distribute users to a subset of ingesters. #1947
  * `-experimental.distributor.user-subring-size`
* [FEATURE] Add flag `-experimental.tsdb.stripe-size` to expose TSDB stripe size option. #2185
* [FEATURE] Experimental Delete Series: Added support for Deleting Series with Prometheus style API. Needs to be enabled first by setting `-purger.enable` to `true`. Deletion only supported when using `boltdb` and `filesystem` as index and object store respectively. Support for other stores to follow in separate PRs #2103
* [ENHANCEMENT] Alertmanager: Expose Per-tenant alertmanager metrics #2124
* [ENHANCEMENT] Add `status` label to `cortex_alertmanager_configs` metric to gauge the number of valid and invalid configs. #2125
* [ENHANCEMENT] Cassandra Authentication: added the `custom_authenticators` config option that allows users to authenticate with cassandra clusters using password authenticators that are not approved by default in [gocql](https://github.com/gocql/gocql/blob/81b8263d9fe526782a588ef94d3fa5c6148e5d67/conn.go#L27) #2093
* [ENHANCEMENT] Cassandra Storage: added `max_retries`, `retry_min_backoff` and `retry_max_backoff` configuration options to enable retrying recoverable errors. #2054
* [ENHANCEMENT] Allow to configure HTTP and gRPC server listen address, maximum number of simultaneous connections and connection keepalive settings.
  * `-server.http-listen-address`
  * `-server.http-conn-limit`
  * `-server.grpc-listen-address`
  * `-server.grpc-conn-limit`
  * `-server.grpc.keepalive.max-connection-idle`
  * `-server.grpc.keepalive.max-connection-age`
  * `-server.grpc.keepalive.max-connection-age-grace`
  * `-server.grpc.keepalive.time`
  * `-server.grpc.keepalive.timeout`
* [ENHANCEMENT] PostgreSQL: Bump up `github.com/lib/pq` from `v1.0.0` to `v1.3.0` to support PostgreSQL SCRAM-SHA-256 authentication. #2097
* [ENHANCEMENT] Cassandra Storage: User no longer need `CREATE` privilege on `<all keyspaces>` if given keyspace exists. #2032
* [ENHANCEMENT] Cassandra Storage: added `password_file` configuration options to enable reading Cassandra password from file. #2096
* [ENHANCEMENT] Configs API: Allow GET/POST configs in YAML format. #2181
* [ENHANCEMENT] Background cache writes are batched to improve parallelism and observability. #2135
* [ENHANCEMENT] Add automatic repair for checkpoint and WAL. #2105
* [ENHANCEMENT] Support `lastEvaluation` and `evaluationTime` in `/api/v1/rules` endpoints and make order of groups stable. #2196
* [ENHANCEMENT] Skip expired requests in query-frontend scheduling. #2082
* [ENHANCEMENT] Add ability to configure gRPC keepalive settings. #2066
* [ENHANCEMENT] Experimental TSDB: Export TSDB Syncer metrics from Compactor component, they are prefixed with `cortex_compactor_`. #2023
* [ENHANCEMENT] Experimental TSDB: Added dedicated flag `-experimental.tsdb.bucket-store.tenant-sync-concurrency` to configure the maximum number of concurrent tenants for which blocks are synched. #2026
* [ENHANCEMENT] Experimental TSDB: Expose metrics for objstore operations (prefixed with `cortex_<component>_thanos_objstore_`, component being one of `ingester`, `querier` and `compactor`). #2027
* [ENHANCEMENT] Experimental TSDB: Added support for Azure Storage to be used for block storage, in addition to S3 and GCS. #2083
* [ENHANCEMENT] Experimental TSDB: Reduced memory allocations in the ingesters when using the experimental blocks storage. #2057
* [ENHANCEMENT] Experimental Memberlist KV: expose `-memberlist.gossip-to-dead-nodes-time` and `-memberlist.dead-node-reclaim-time` options to control how memberlist library handles dead nodes and name reuse. #2131
* [BUGFIX] Alertmanager: fixed panic upon applying a new config, caused by duplicate metrics registration in the `NewPipelineBuilder` function. #211
* [BUGFIX] Azure Blob ChunkStore: Fixed issue causing `invalid chunk checksum` errors. #2074
* [BUGFIX] The gauge `cortex_overrides_last_reload_successful` is now only exported by components that use a `RuntimeConfigManager`. Previously, for components that do not initialize a `RuntimeConfigManager` (such as the compactor) the gauge was initialized with 0 (indicating error state) and then never updated, resulting in a false-negative permanent error state. #2092
* [BUGFIX] Fixed WAL metric names, added the `cortex_` prefix.
* [BUGFIX] Restored histogram `cortex_configs_request_duration_seconds` #2138
* [BUGFIX] Fix wrong syntax for `url` in config-file-reference. #2148
* [BUGFIX] Fixed some 5xx status code returned by the query-frontend when they should actually be 4xx. #2122
* [BUGFIX] Fixed leaked goroutines in the querier. #2070
* [BUGFIX] Experimental TSDB: fixed `/all_user_stats` and `/api/prom/user_stats` endpoints when using the experimental TSDB blocks storage. #2042
* [BUGFIX] Experimental TSDB: fixed ruler to correctly work with the experimental TSDB blocks storage. #2101

### Changes to denormalised tokens in the ring

Cortex 0.4.0 is the last version that can *write* denormalised tokens. Cortex 0.5.0 and above always write normalised tokens.

Cortex 0.6.0 is the last version that can *read* denormalised tokens. Starting with Cortex 0.7.0 only normalised tokens are supported, and ingesters writing denormalised tokens to the ring (running Cortex 0.4.0 or earlier with `-ingester.normalise-tokens=false`) are ignored by distributors. Such ingesters should either switch to using normalised tokens, or be upgraded to Cortex 0.5.0 or later.

### Known issues

- The gRPC streaming for ingesters doesn't work when using the experimental TSDB blocks storage. Please do not enable `-querier.ingester-streaming` if you're using the TSDB blocks storage. If you want to enable it, you can build Cortex from `master` given the issue has been fixed after Cortex `0.7` branch has been cut and the fix wasn't included in the `0.7` because related to an experimental feature.

### Annotated config file breaking changes

In this section you can find a config file diff showing the breaking changes introduced in Cortex `0.7`. You can also find the [full configuration file reference doc](https://cortexmetrics.io/docs/configuration/configuration-file/) in the website.

 ```diff
### Root level config

 # "configdb" has been moved to "alertmanager > store > configdb".
-[configdb: <configdb_config>]

 # "config_store" has been renamed to "configs".
-[config_store: <configstore_config>]
+[configs: <configs_config>]


### `distributor_config`

 # The support to hook an external billing system has been removed.
-[enable_billing: <boolean> | default = false]
-billing:
-  [maxbufferedevents: <int> | default = 1024]
-  [retrydelay: <duration> | default = 500ms]
-  [ingesterhostport: <string> | default = "localhost:24225"]


### `querier_config`

 # "ingestermaxquerylookback" has been renamed to "query_ingesters_within".
-[ingestermaxquerylookback: <duration> | default = 0s]
+[query_ingesters_within: <duration> | default = 0s]


### `queryrange_config`

results_cache:
  cache:
     # "defaul_validity" has been renamed to "default_validity".
-    [defaul_validity: <duration> | default = 0s]
+    [default_validity: <duration> | default = 0s]

   # "cache_split_interval" has been deprecated in favor of "split_queries_by_interval".
-  [cache_split_interval: <duration> | default = 24h0m0s]


### `alertmanager_config`

# The "store" config block has been added. This includes "configdb" which previously
# was the "configdb" root level config block.
+store:
+  [type: <string> | default = "configdb"]
+  [configdb: <configstore_config>]
+  local:
+    [path: <string> | default = ""]


### `storage_config`

index_queries_cache_config:
   # "defaul_validity" has been renamed to "default_validity".
-  [defaul_validity: <duration> | default = 0s]
+  [default_validity: <duration> | default = 0s]


### `chunk_store_config`

chunk_cache_config:
   # "defaul_validity" has been renamed to "default_validity".
-  [defaul_validity: <duration> | default = 0s]
+  [default_validity: <duration> | default = 0s]

write_dedupe_cache_config:
   # "defaul_validity" has been renamed to "default_validity".
-  [defaul_validity: <duration> | default = 0s]
+  [default_validity: <duration> | default = 0s]

 # "min_chunk_age" has been removed in favor of "querier > query_store_after".
-[min_chunk_age: <duration> | default = 0s]


### `configs_config`

-# "uri" has been moved to "database > uri".
-[uri: <string> | default = "postgres://postgres@configs-db.weave.local/configs?sslmode=disable"]

-# "migrationsdir" has been moved to "database > migrations_dir".
-[migrationsdir: <string> | default = ""]

-# "passwordfile" has been moved to "database > password_file".
-[passwordfile: <string> | default = ""]

+database:
+  [uri: <string> | default = "postgres://postgres@configs-db.weave.local/configs?sslmode=disable"]
+  [migrations_dir: <string> | default = ""]
+  [password_file: <string> | default = ""]
```

## 0.6.1 / 2020-02-05

* [BUGFIX] Fixed parsing of the WAL configuration when specified in the YAML config file. #2071

## 0.6.0 / 2020-01-28

Note that the ruler flags need to be changed in this upgrade. You're moving from a single node ruler to something that might need to be sharded.
Further, if you're using the configs service, we've upgraded the migration library and this requires some manual intervention. See full instructions below to upgrade your PostgreSQL.

* [CHANGE] The frontend component now does not cache results if it finds a `Cache-Control` header and if one of its values is `no-store`. #1974
* [CHANGE] Flags changed with transition to upstream Prometheus rules manager:
  * `-ruler.client-timeout` is now `ruler.configs.client-timeout` in order to match `ruler.configs.url`.
  * `-ruler.group-timeout`has been removed.
  * `-ruler.num-workers` has been removed.
  * `-ruler.rule-path` has been added to specify where the prometheus rule manager will sync rule files.
  * `-ruler.storage.type` has beem added to specify the rule store backend type, currently only the configdb.
  * `-ruler.poll-interval` has been added to specify the interval in which to poll new rule groups.
  * `-ruler.evaluation-interval` default value has changed from `15s` to `1m` to match the default evaluation interval in Prometheus.
  * Ruler sharding requires a ring which can be configured via the ring flags prefixed by `ruler.ring.`. #1987
* [CHANGE] Use relative links from /ring page to make it work when used behind reverse proxy. #1896
* [CHANGE] Deprecated `-distributor.limiter-reload-period` flag. #1766
* [CHANGE] Ingesters now write only normalised tokens to the ring, although they can still read denormalised tokens used by other ingesters. `-ingester.normalise-tokens` is now deprecated, and ignored. If you want to switch back to using denormalised tokens, you need to downgrade to Cortex 0.4.0. Previous versions don't handle claiming tokens from normalised ingesters correctly. #1809
* [CHANGE] Overrides mechanism has been renamed to "runtime config", and is now separate from limits. Runtime config is simply a file that is reloaded by Cortex every couple of seconds. Limits and now also multi KV use this mechanism.<br />New arguments were introduced: `-runtime-config.file` (defaults to empty) and `-runtime-config.reload-period` (defaults to 10 seconds), which replace previously used `-limits.per-user-override-config` and `-limits.per-user-override-period` options. Old options are still used if `-runtime-config.file` is not specified. This change is also reflected in YAML configuration, where old `limits.per_tenant_override_config` and `limits.per_tenant_override_period` fields are replaced with `runtime_config.file` and `runtime_config.period` respectively. #1749
* [CHANGE] Cortex now rejects data with duplicate labels. Previously, such data was accepted, with duplicate labels removed with only one value left. #1964
* [CHANGE] Changed the default value for `-distributor.ha-tracker.prefix` from `collectors/` to `ha-tracker/` in order to not clash with other keys (ie. ring) stored in the same key-value store. #1940
* [FEATURE] Experimental: Write-Ahead-Log added in ingesters for more data reliability against ingester crashes. #1103
  * `--ingester.wal-enabled`: Setting this to `true` enables writing to WAL during ingestion.
  * `--ingester.wal-dir`: Directory where the WAL data should be stored and/or recovered from.
  * `--ingester.checkpoint-enabled`: Set this to `true` to enable checkpointing of in-memory chunks to disk.
  * `--ingester.checkpoint-duration`: This is the interval at which checkpoints should be created.
  * `--ingester.recover-from-wal`: Set this to `true` to recover data from an existing WAL.
  * For more information, please checkout the ["Ingesters with WAL" guide](https://cortexmetrics.io/docs/guides/ingesters-with-wal/).
* [FEATURE] The distributor can now drop labels from samples (similar to the removal of the replica label for HA ingestion) per user via the `distributor.drop-label` flag. #1726
* [FEATURE] Added flag `debug.mutex-profile-fraction` to enable mutex profiling #1969
* [FEATURE] Added `global` ingestion rate limiter strategy. Deprecated `-distributor.limiter-reload-period` flag. #1766
* [FEATURE] Added support for Microsoft Azure blob storage to be used for storing chunk data. #1913
* [FEATURE] Added readiness probe endpoint`/ready` to queriers. #1934
* [FEATURE] Added "multi" KV store that can interact with two other KV stores, primary one for all reads and writes, and secondary one, which only receives writes. Primary/secondary store can be modified in runtime via runtime-config mechanism (previously "overrides"). #1749
* [FEATURE] Added support to store ring tokens to a file and read it back on startup, instead of generating/fetching the tokens to/from the ring. This feature can be enabled with the flag `-ingester.tokens-file-path`. #1750
* [FEATURE] Experimental TSDB: Added `/series` API endpoint support with TSDB blocks storage. #1830
* [FEATURE] Experimental TSDB: Added TSDB blocks `compactor` component, which iterates over users blocks stored in the bucket and compact them according to the configured block ranges. #1942
* [ENHANCEMENT] metric `cortex_ingester_flush_reasons` gets a new `reason` value: `Spread`, when `-ingester.spread-flushes` option is enabled. #1978
* [ENHANCEMENT] Added `password` and `enable_tls` options to redis cache configuration. Enables usage of Microsoft Azure Cache for Redis service. #1923
* [ENHANCEMENT] Upgraded Kubernetes API version for deployments from `extensions/v1beta1` to `apps/v1`. #1941
* [ENHANCEMENT] Experimental TSDB: Open existing TSDB on startup to prevent ingester from becoming ready before it can accept writes. The max concurrency is set via `--experimental.tsdb.max-tsdb-opening-concurrency-on-startup`. #1917
* [ENHANCEMENT] Experimental TSDB: Querier now exports aggregate metrics from Thanos bucket store and in memory index cache (many metrics to list, but all have `cortex_querier_bucket_store_` or `cortex_querier_blocks_index_cache_` prefix). #1996
* [ENHANCEMENT] Experimental TSDB: Improved multi-tenant bucket store. #1991
  * Allowed to configure the blocks sync interval via `-experimental.tsdb.bucket-store.sync-interval` (0 disables the sync)
  * Limited the number of tenants concurrently synched by `-experimental.tsdb.bucket-store.block-sync-concurrency`
  * Renamed `cortex_querier_sync_seconds` metric to `cortex_querier_blocks_sync_seconds`
  * Track `cortex_querier_blocks_sync_seconds` metric for the initial sync too
* [BUGFIX] Fixed unnecessary CAS operations done by the HA tracker when the jitter is enabled. #1861
* [BUGFIX] Fixed ingesters getting stuck in a LEAVING state after coming up from an ungraceful exit. #1921
* [BUGFIX] Reduce memory usage when ingester Push() errors. #1922
* [BUGFIX] Table Manager: Fixed calculation of expected tables and creation of tables from next active schema considering grace period. #1976
* [BUGFIX] Experimental TSDB: Fixed ingesters consistency during hand-over when using experimental TSDB blocks storage. #1854 #1818
* [BUGFIX] Experimental TSDB: Fixed metrics when using experimental TSDB blocks storage. #1981 #1982 #1990 #1983
* [BUGFIX] Experimental memberlist: Use the advertised address when sending packets to other peers of the Gossip memberlist. #1857
* [BUGFIX] Experimental TSDB: Fixed incorrect query results introduced in #2604 caused by a buffer incorrectly reused while iterating samples. #2697

### Upgrading PostgreSQL (if you're using configs service)

Reference: <https://github.com/golang-migrate/migrate/tree/master/database/postgres#upgrading-from-v1>

1. Install the migrate package cli tool: <https://github.com/golang-migrate/migrate/tree/master/cmd/migrate#installation>
2. Drop the `schema_migrations` table: `DROP TABLE schema_migrations;`.
2. Run the migrate command:

```bash
migrate  -path <absolute_path_to_cortex>/cmd/cortex/migrations -database postgres://localhost:5432/database force 2
```

### Known issues

- The `cortex_prometheus_rule_group_last_evaluation_timestamp_seconds` metric, tracked by the ruler, is not unregistered for rule groups not being used anymore. This issue will be fixed in the next Cortex release (see [2033](https://github.com/cortexproject/cortex/issues/2033)).

- Write-Ahead-Log (WAL) does not have automatic repair of corrupt checkpoint or WAL segments, which is possible if ingester crashes abruptly or the underlying disk corrupts. Currently the only way to resolve this is to manually delete the affected checkpoint and/or WAL segments. Automatic repair will be added in the future releases.

## 0.4.0 / 2019-12-02

* [CHANGE] The frontend component has been refactored to be easier to reuse. When upgrading the frontend, cache entries will be discarded and re-created with the new protobuf schema. #1734
* [CHANGE] Removed direct DB/API access from the ruler. `-ruler.configs.url` has been now deprecated. #1579
* [CHANGE] Removed `Delta` encoding. Any old chunks with `Delta` encoding cannot be read anymore. If `ingester.chunk-encoding` is set to `Delta` the ingester will fail to start. #1706
* [CHANGE] Setting `-ingester.max-transfer-retries` to 0 now disables hand-over when ingester is shutting down. Previously, zero meant infinite number of attempts. #1771
* [CHANGE] `dynamo` has been removed as a valid storage name to make it consistent for all components. `aws` and `aws-dynamo` remain as valid storage names.
* [CHANGE/FEATURE] The frontend split and cache intervals can now be configured using the respective flag `--querier.split-queries-by-interval` and `--frontend.cache-split-interval`.
  * If `--querier.split-queries-by-interval` is not provided request splitting is disabled by default.
  * __`--querier.split-queries-by-day` is still accepted for backward compatibility but has been deprecated. You should now use `--querier.split-queries-by-interval`. We recommend a to use a multiple of 24 hours.__
* [FEATURE] Global limit on the max series per user and metric #1760
  * `-ingester.max-global-series-per-user`
  * `-ingester.max-global-series-per-metric`
  * Requires `-distributor.replication-factor` and `-distributor.shard-by-all-labels` set for the ingesters too
* [FEATURE] Flush chunks with stale markers early with `ingester.max-stale-chunk-idle`. #1759
* [FEATURE] EXPERIMENTAL: Added new KV Store backend based on memberlist library. Components can gossip about tokens and ingester states, instead of using Consul or Etcd. #1721
* [FEATURE] EXPERIMENTAL: Use TSDB in the ingesters & flush blocks to S3/GCS ala Thanos. This will let us use an Object Store more efficiently and reduce costs. #1695
* [FEATURE] Allow Query Frontend to log slow queries with `frontend.log-queries-longer-than`. #1744
* [FEATURE] Add HTTP handler to trigger ingester flush & shutdown - used when running as a stateful set with the WAL enabled.  #1746
* [FEATURE] EXPERIMENTAL: Added GCS support to TSDB blocks storage. #1772
* [ENHANCEMENT] Reduce memory allocations in the write path. #1706
* [ENHANCEMENT] Consul client now follows recommended practices for blocking queries wrt returned Index value. #1708
* [ENHANCEMENT] Consul client can optionally rate-limit itself during Watch (used e.g. by ring watchers) and WatchPrefix (used by HA feature) operations. Rate limiting is disabled by default. New flags added: `--consul.watch-rate-limit`, and `--consul.watch-burst-size`. #1708
* [ENHANCEMENT] Added jitter to HA deduping heartbeats, configure using `distributor.ha-tracker.update-timeout-jitter-max` #1534
* [ENHANCEMENT] Add ability to flush chunks with stale markers early. #1759
* [BUGFIX] Stop reporting successful actions as 500 errors in KV store metrics. #1798
* [BUGFIX] Fix bug where duplicate labels can be returned through metadata APIs. #1790
* [BUGFIX] Fix reading of old, v3 chunk data. #1779
* [BUGFIX] Now support IAM roles in service accounts in AWS EKS. #1803
* [BUGFIX] Fixed duplicated series returned when querying both ingesters and store with the experimental TSDB blocks storage. #1778

In this release we updated the following dependencies:

- gRPC v1.25.0  (resulted in a drop of 30% CPU usage when compression is on)
- jaeger-client v2.20.0
- aws-sdk-go to v1.25.22

## 0.3.0 / 2019-10-11

This release adds support for Redis as an alternative to Memcached, and also includes many optimisations which reduce CPU and memory usage.

* [CHANGE] Gauge metrics were renamed to drop the `_total` suffix. #1685
  * In Alertmanager, `alertmanager_configs_total` is now `alertmanager_configs`
  * In Ruler, `scheduler_configs_total` is now `scheduler_configs`
  * `scheduler_groups_total` is now `scheduler_groups`.
* [CHANGE] `--alertmanager.configs.auto-slack-root` flag was dropped as auto Slack root is not supported anymore. #1597
* [CHANGE] In table-manager, default DynamoDB capacity was reduced from 3,000 units to 1,000 units. We recommend you do not run with the defaults: find out what figures are needed for your environment and set that via `-dynamodb.periodic-table.write-throughput` and `-dynamodb.chunk-table.write-throughput`.
* [FEATURE] Add Redis support for caching #1612
* [FEATURE] Allow spreading chunk writes across multiple S3 buckets #1625
* [FEATURE] Added `/shutdown` endpoint for ingester to shutdown all operations of the ingester. #1746
* [ENHANCEMENT] Upgraded Prometheus to 2.12.0 and Alertmanager to 0.19.0. #1597
* [ENHANCEMENT] Cortex is now built with Go 1.13 #1675, #1676, #1679
* [ENHANCEMENT] Many optimisations, mostly impacting ingester and querier: #1574, #1624, #1638, #1644, #1649, #1654, #1702

Full list of changes: <https://github.com/cortexproject/cortex/compare/v0.2.0...v0.3.0>

## 0.2.0 / 2019-09-05

This release has several exciting features, the most notable of them being setting `-ingester.spread-flushes` to potentially reduce your storage space by upto 50%.

* [CHANGE] Flags changed due to changes upstream in Prometheus Alertmanager #929:
  * `alertmanager.mesh.listen-address` is now `cluster.listen-address`
  * `alertmanager.mesh.peer.host` and `alertmanager.mesh.peer.service` can be replaced by `cluster.peer`
  * `alertmanager.mesh.hardware-address`, `alertmanager.mesh.nickname`, `alertmanager.mesh.password`, and `alertmanager.mesh.peer.refresh-interval` all disappear.
* [CHANGE] --claim-on-rollout flag deprecated; feature is now always on #1566
* [CHANGE] Retention period must now be a multiple of periodic table duration #1564
* [CHANGE] The value for the name label for the chunks memcache in all `cortex_cache_` metrics is now `chunksmemcache` (before it was `memcache`) #1569
* [FEATURE] Makes the ingester flush each timeseries at a specific point in the max-chunk-age cycle with `-ingester.spread-flushes`. This means multiple replicas of a chunk are very likely to contain the same contents which cuts chunk storage space by up to 66%. #1578
* [FEATURE] Make minimum number of chunk samples configurable per user #1620
* [FEATURE] Honor HTTPS for custom S3 URLs #1603
* [FEATURE] You can now point the query-frontend at a normal Prometheus for parallelisation and caching #1441
* [FEATURE] You can now specify `http_config` on alert receivers #929
* [FEATURE] Add option to use jump hashing to load balance requests to memcached #1554
* [FEATURE] Add status page for HA tracker to distributors #1546
* [FEATURE] The distributor ring page is now easier to read with alternate rows grayed out #1621

## 0.1.0 / 2019-08-07

* [CHANGE] HA Tracker flags were renamed to provide more clarity #1465
  * `distributor.accept-ha-labels` is now `distributor.ha-tracker.enable`
  * `distributor.accept-ha-samples` is now `distributor.ha-tracker.enable-for-all-users`
  * `ha-tracker.replica` is now `distributor.ha-tracker.replica`
  * `ha-tracker.cluster` is now `distributor.ha-tracker.cluster`
* [FEATURE] You can specify "heap ballast" to reduce Go GC Churn #1489
* [BUGFIX] HA Tracker no longer always makes a request to Consul/Etcd when a request is not from the active replica #1516
* [BUGFIX] Queries are now correctly cancelled by the query-frontend #1508
