#!/usr/bin/env bash
# Trusted, disposable CI only: downgrade this test tenant's chunk schema.
set -euo pipefail
[[ ${SEEKDB_LEGACY_DISPOSABLE:-} == 1 ]] || { echo 'Requires SEEKDB_LEGACY_DISPOSABLE=1'; exit 1; }
action=${1:?}; kb=${2:?}; doc=${3:?}; neighbor=${4:?}
[[ $action == downgrade || $action == verify ]]
for id in "$kb" "$doc" "$neighbor"; do [[ $id =~ ^[a-fA-F0-9]{32}$ ]] || exit 1; done
runtime=${SEEKDB_LEGACY_RUNTIME:?absolute runtime directory required}
[[ $runtime == /* && -f $runtime/api.override.yml ]]
compose=(docker compose --env-file "$runtime/.env" --project-directory "$runtime" -f "$runtime/docker-compose.yml" -f "$runtime/api.override.yml" -p "${SEEKDB_LEGACY_PROJECT:-seekdb-api}")
metadata() { "${compose[@]}" exec -T mysql sh -c 'MYSQL_PWD="$MYSQL_PASSWORD" exec mysql -uroot --batch --skip-column-names "${MYSQL_DBNAME:-rag_flow}"'; }
seekdb() { "${compose[@]}" exec -T seekdb sh -c 'MYSQL_PWD="$SEEKDB_PASSWORD" exec mysql -h127.0.0.1 -P2881 -uroot --batch --skip-column-names "${SEEKDB_DOC_DBNAME:-ragflow_doc}"'; }
tenant=$(metadata <<< "SELECT tenant_id FROM knowledgebase WHERE id='$kb';")
[[ $tenant =~ ^[a-fA-F0-9]{32}$ ]] || { echo 'Cannot uniquely resolve tenant'; exit 1; }
table="ragflow_$tenant"
# Refuse to alter a shared data set or a tenant with another test's chunks.
foreign=$(seekdb <<< "SELECT COUNT(*) FROM \`$table\` WHERE kb_id IS NULL OR doc_id IS NULL OR kb_id <> '$kb' OR doc_id NOT IN ('$doc','$neighbor');")
[[ $foreign == 0 ]] || { echo 'Refusing shared tenant fixture'; exit 1; }
for id in "$doc" "$neighbor"; do
 count=$(seekdb <<< "SELECT COUNT(*) FROM \`$table\` WHERE kb_id='$kb' AND doc_id='$id';")
 [[ $count =~ ^[1-9][0-9]*$ ]] || { echo 'Expected worker-generated chunks for both documents'; exit 1; }
done
missing=$(seekdb <<< "SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='$table' AND COLUMN_NAME IN ('compile_kwd','compilation_template_kind_kwd');")
if [[ $action == verify ]]; then
 [[ $missing == 0 ]] || { echo 'Worker unexpectedly migrated optional columns'; exit 1; }
 old=$(seekdb <<< "SELECT COUNT(*) FROM \`$table\` WHERE id LIKE 'legacy\\_old\\_%' OR content_with_weight LIKE '%Obsoletelegacyrevision%';")
 [[ $old == 0 ]] || { echo 'Legacy indexed rows survived'; exit 1; }
 echo 'Verified old schema retained and legacy rows absent'
 exit 0
fi
[[ $missing == 2 ]] || { echo 'Expected fresh baseline schema before downgrade'; exit 1; }
# These source rows have no compiler products; persist source classification in
# extra before removing the newly introduced physical columns.
seekdb <<SQL
UPDATE \`$table\` SET extra=JSON_MERGE_PATCH(COALESCE(extra,JSON_OBJECT()),JSON_OBJECT('compile_kwd','','compilation_template_kind_kwd','','fixture_version','pre_22121')) WHERE kb_id='$kb';
SQL
# Fixed #22121 added-column manifest. Do not derive a mutation list from PR code.
optional='mom name name_kwd important_kwd_empty_count mention_count_int rank_int mom_with_weight toc_kwd deleted_doc_id compile_kwd compilation_template_kind_kwd plan_kwd scope_kwd source_chunk_ids source_doc_ids compilation_template_ids chunk_hash_kwd input_hash_kwd wiki_slug_kwd summary_with_weight skill_with_weight skill_kwd children_kwd doc_ids_kwd slug_kwd title_kwd topic_kwd plan_group_kwd generation_kwd page_type_kwd entity_names_kwd entity_names outlinks_kwd related_kb_pages_kwd page_version_int synthesis_version_int claims page_ids page_id source_chunk_hashes map_checksum from_kwd to_kwd rechunk_kwd rechunked_from_template_id rechunked_from_chunk_ids superseded_by_chunk_id doc_count_int depth_int parent_kwd outlinks_int token_num aliases aliases_flat_kwd'
for column in $optional; do
 present=$(seekdb <<< "SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='$table' AND COLUMN_NAME='$column';")
 if [[ $present == 1 ]]; then seekdb <<< "ALTER TABLE \`$table\` DROP COLUMN \`$column\`;"; fi
done
# Preserve uploaded bytes, metadata, completed task and neighbor. Only replace
# target index IDs/text, representing an obsolete stored revision.
seekdb <<SQL
UPDATE \`$table\` SET id=CONCAT('legacy_old_',id),content_with_weight='Obsoletelegacyrevision stale violet roots.',content_ltks='Obsoletelegacyrevision stale violet roots',content_sm_ltks='Obsoletelegacyrevision stale violet roots' WHERE kb_id='$kb' AND doc_id='$doc';
SQL
count=$(seekdb <<< "SELECT COUNT(*) FROM \`$table\` WHERE doc_id='$doc' AND id LIKE 'legacy\\_old\\_%';")
[[ $count =~ ^[1-9][0-9]*$ ]] || { echo 'Fixture mutation did not produce old rows'; exit 1; }
echo "Prepared $count obsolete chunks in fixed pre-22121 schema"
