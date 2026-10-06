"""Owned PG123 + actual Python3.6 CORE_ONLY tests, never enrollment acceptance.

Host harness ignores inherited DB/container variables. Child shares ONLY the
network-none namespace of its own disposable PG and verifies an ownership token
before resetting synthetic rows. Missing tools/runtime/DB are FAIL, never SKIP.
"""
import copy
import hashlib
import json
import os
from pathlib import Path
import py_compile
import re
import shutil
import subprocess
import sys
import tempfile
import types
import unittest
import uuid

ROOT = Path(__file__).resolve().parent.parent
sys.dont_write_bytecode = True


def source_module(name, path):
    module = types.ModuleType(name)
    module.__file__ = str(path)
    exec(compile(path.read_bytes(), str(path), 'exec'), module.__dict__)
    return module


core = source_module('live_core', ROOT / 'ops/quarantine-live-snapshot.py')
enroll = source_module('enroll', ROOT / 'ops/enroll-quarantine.py')
approval_mod = source_module('approval_mod', ROOT / 'ops/quarantine-approval.py')



class CoreOnlyPostgresTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        if sys.version_info[:2] != (3, 6) or not os.environ.get('ISSUE203_OWNED_LIVE_DB'):
            raise RuntimeError('required owned Python3.6 + PG unavailable; not SKIP')
        import psycopg2
        cls.db = psycopg2.connect(host='127.0.0.1', dbname='xianzhi_test', user='postgres', password='test_secret_pass')
        cls.db.autocommit = True
        with cls.db.cursor() as cursor:
            cursor.execute('SELECT version()')
            print('ACTUAL_POSTGRES ' + cursor.fetchone()[0])
            cursor.execute('SELECT token FROM public.issue203_owned_live_test')
            if cursor.fetchall() != [(os.environ['ISSUE203_OWNED_LIVE_DB'],)]:
                raise RuntimeError('refuse unowned database')
        approval_tests = source_module('approval_tests', ROOT / 'tests/issue203-approval-test.py')
        cls.approval_module = approval_tests.approval
        cls.approval_class = approval_tests.ApprovalTests
        cls.approval_class.setUpClass()  # synthetic disposable root only
        core._approval.REGISTRY_PATH = cls.approval_module.REGISTRY_PATH
        enroll.approval.REGISTRY_PATH = cls.approval_module.REGISTRY_PATH
        enroll.live_snapshot._approval.REGISTRY_PATH = cls.approval_module.REGISTRY_PATH
        print('RUNTIME Python=%s PG=123+real-schema CORE_ONLY; skip0' % sys.version.split()[0])

    @classmethod
    def tearDownClass(cls):
        cls.approval_class.tearDownClass()
        cls.db.close()

    def setUp(self):
        with self.db.cursor() as cursor:
            cursor.execute('TRUNCATE public.xz_assets,public.xz_file_objects,public.xz_storage_configs,public.xz_file_relations,public.xz_storage_jobs,public.xz_multipart_uploads CASCADE')
            cursor.execute('TRUNCATE public.provider_execution_correlations,public.provider_executions,public.xz_generation_tasks,public.xz_personal_point_reservations,public.xz_point_accounts,public.xz_user_wallets,public.xz_wallet_ledger,public.xz_billing_lifecycle_events,public.xz_billing_events CASCADE')
            for i in range(9):
                tid, uid, aid = 'synthetic-task-%d' % i, 'synthetic-user-%d' % i, 'synthetic-account-%d' % i
                cursor.execute('INSERT INTO public.xz_point_accounts(id,user_id,available,frozen) VALUES(%s,%s,100,5)', (aid, uid))
                cursor.execute('INSERT INTO public.xz_user_wallets(user_id,token_balance,frozen_token,total_token_granted) VALUES(%s,100,5,105)', (uid,))
                raw = {}
                ledger_key = tid + ':RESERVE'
                if i < 6:
                    rid, lot = 'synthetic-reservation-%d' % i, 'synthetic-lot-%d' % i
                    reserve_key = 'generation:reserve:' + tid
                    cursor.execute("INSERT INTO public.xz_personal_point_reservations(id,account_id,user_id,business_type,business_id,requested_points,reserved_points,idempotency_key) VALUES(%s,%s,%s,'GENERATION_TASK',%s,5,5,%s)", (rid, aid, uid, tid, reserve_key))
                    cursor.execute("INSERT INTO public.xz_personal_point_lots(id,account_id,user_id,source_type,original_points,available_points,reserved_points,idempotency_key) VALUES(%s,%s,%s,'RECHARGE',105,100,5,%s)", (lot,aid,uid,'synthetic-grant-'+str(i)))
                    cursor.execute('INSERT INTO public.xz_personal_point_reservation_allocations(id,reservation_id,lot_id,account_id,user_id,allocated_points,reserved_points) VALUES(%s,%s,%s,%s,%s,5,5)', ('synthetic-allocation-'+str(i),rid,lot,aid,uid))
                    cursor.execute("INSERT INTO public.xz_personal_point_lot_movements(id,lot_id,account_id,user_id,movement_type,points,available_before,available_after,reserved_before,reserved_after,consumed_before,consumed_after,expired_before,expired_after,reversed_before,reversed_after,reservation_id,idempotency_key) VALUES(%s,%s,%s,%s,'RESERVE',5,105,100,0,5,0,0,0,0,0,0,%s,%s)", ('synthetic-movement-'+str(i),lot,aid,uid,rid,'reserve:'+reserve_key+':'+lot))
                    raw = {'billingEngine': 'PERSONAL_LOT_V1', 'personalPointAccountId': aid, 'personalPointReservationId': rid}
                    ledger_key = 'personal-point:reserve:' + aid + ':' + reserve_key
                cursor.execute("INSERT INTO public.xz_wallet_ledger(id,account_id,user_id,task_id,reference_type,reference_id,entry_type,points,available_before,available_after,frozen_before,frozen_after,idempotency_key) VALUES(%s,%s,%s,%s,'GENERATION_TASK',%s,'RESERVE',5,105,100,0,5,%s)", ('synthetic-ledger-'+str(i),aid,uid,tid,tid,ledger_key))
                for kind, status in [('QUOTE','QUOTED'),('RESERVE','RESERVED')]:
                    cursor.execute('INSERT INTO public.xz_billing_lifecycle_events(id,task_id,user_id,event_type,billing_status,points,idempotency_key) VALUES(%s,%s,%s,%s,%s,5,%s)', ('synthetic-event-'+str(i)+kind,tid,uid,kind,status,tid+':'+kind))
                params = {'url': 'https://synthetic.invalid/private', 'billingReserved': True, 'billingReservationPointCost': 5}
                cursor.execute("INSERT INTO public.xz_generation_tasks(id,user_id,type,model,status,task_status,execution_generation,prompt,params,raw,point_cost,reserved_points,billing_status) VALUES(%s,%s,'TEXT_TO_IMAGE','synthetic-model','FAILED','FAILED',2,%s,%s::jsonb,%s::jsonb,5,5,'RESERVED')", (tid, uid, 'SENSITIVE_SYNTHETIC_PROMPT', json.dumps(params), json.dumps(raw)))
                # PERSONAL authorization is the user identity, independently of
                # the point-account marker/PK (enterprise_runtime + ai_capability).
                cursor.execute("UPDATE public.xz_generation_tasks SET billing_account_id=user_id,params=params||jsonb_build_object('billing_account_id',user_id),raw=raw||jsonb_build_object('billingAccountId',user_id) WHERE id=%s", (tid,))
                if i >= 6:
                    # Historical wallet reserve/release preceded migration103.
                    # The post-settlement balance becomes a LEGACY lot + OPENING,
                    # not a modern reservation or a second economic reserve.
                    cursor.execute("UPDATE public.xz_point_accounts SET available=105,frozen=0 WHERE id=%s", (aid,))
                    cursor.execute("UPDATE public.xz_user_wallets SET token_balance=105,frozen_token=0 WHERE user_id=%s", (uid,))
                    cursor.execute("UPDATE public.xz_generation_tasks SET billing_status='RELEASED',released_points=5,params=params||'{\"billingRefunded\":true}'::jsonb WHERE id=%s", (tid,))
                    cursor.execute("INSERT INTO public.xz_wallet_ledger(id,account_id,user_id,task_id,reference_type,reference_id,entry_type,points,available_before,available_after,frozen_before,frozen_after,idempotency_key) VALUES(%s,%s,%s,%s,'GENERATION_TASK',%s,'RELEASE',5,100,105,5,0,%s)", ('synthetic-release-'+str(i),aid,uid,tid,tid,tid+':RELEASE'))
                    cursor.execute("INSERT INTO public.xz_billing_lifecycle_events(id,task_id,user_id,event_type,billing_status,points,idempotency_key) VALUES(%s,%s,%s,'RELEASE','RELEASED',5,%s)", ('synthetic-event-'+str(i)+'RELEASE',tid,uid,tid+':RELEASE'))
                    lot='personal_point_lot_legacy_'+hashlib.md5(aid.encode()).hexdigest()[:24]
                    cursor.execute("INSERT INTO public.xz_personal_point_lots(id,account_id,user_id,source_type,reference_type,reference_id,original_points,available_points,reserved_points,policy_snapshot,idempotency_key,status,metadata) VALUES(%s,%s,%s,'LEGACY','POINT_ACCOUNT',%s,105,105,0,%s::jsonb,%s,'LEGACY',%s::jsonb)", (lot,aid,uid,aid,json.dumps(dict(migration='103-personal-gift-point-expiry',permanent=True,legacyAvailable=105,legacyFrozen=0)),'migration:103:legacy:'+aid,json.dumps(dict(migration='103-personal-gift-point-expiry'))))
                    cursor.execute("INSERT INTO public.xz_personal_point_lot_movements(id,lot_id,account_id,user_id,movement_type,points,available_before,available_after,reserved_before,reserved_after,consumed_before,consumed_after,expired_before,expired_after,reversed_before,reversed_after,reference_type,reference_id,idempotency_key,metadata) VALUES(%s,%s,%s,%s,'OPENING',105,0,105,0,0,0,0,0,0,0,0,'POINT_ACCOUNT',%s,%s,%s::jsonb)", ('personal_point_lot_movement_legacy_'+hashlib.md5(aid.encode()).hexdigest()[:24],lot,aid,uid,aid,'migration:103:legacy-opening:'+aid,json.dumps(dict(migration='103-personal-gift-point-expiry'))))
                cursor.execute("INSERT INTO public.provider_executions(id,task_id,attempt,status,task_execution_generation,provider,provider_channel,provider_model,capability,request_fingerprint,provider_request_id) VALUES(%s,%s,1,'unknown',1,'synthetic-provider','synthetic-channel','synthetic-model','image',%s,%s)", (901+i, tid, 'a'*64, 'SENSITIVE_SYNTHETIC_REQUEST'))
                cursor.execute("INSERT INTO public.provider_execution_correlations(id,execution_id,kind,provider_job_id,provider_state) VALUES(%s,%s,'job','SENSITIVE_SYNTHETIC_JOB','possibly_submitted')", (1901+i, 901+i))
            # Prior failed attempt and its correlation MUST be in each task's core.
            cursor.execute("INSERT INTO public.provider_executions(id,task_id,attempt,status,task_execution_generation,provider,provider_channel,provider_model,capability,request_fingerprint) VALUES(800,'synthetic-task-0',2,'failed',1,'synthetic-provider','synthetic-channel','synthetic-model','image',%s)", ('b'*64,))
            cursor.execute("INSERT INTO public.provider_execution_correlations(id,execution_id,kind) VALUES(1800,800,'route')")
        self.entries = [dict(execution_id=901+i, task_id='synthetic-task-%d' % i, attempt=1, generation=1, task_generation=2) for i in range(9)]
        snapshots = core.sample_core_read_only(self.db, self.entries)
        self.hashes = {eid: core.core_sha256(value) for eid, value in snapshots.items()}

    def sql(self, statement, values=()):
        with self.db.cursor() as cursor:
            cursor.execute(statement, values)

    def compare(self, entries=None, hashes=None):
        with self.db.cursor() as cursor:
            cursor.execute('BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY')
            try:
                return core.compare_core_in_transaction(cursor, self.entries if entries is None else entries, self.hashes if hashes is None else hashes)
            finally:
                cursor.execute('ROLLBACK')

    def reject(self, code, entries=None, hashes=None):
        with self.assertRaisesRegex(core.SnapshotError, '^QUARANTINE_LIVE_' + code + '$'):
            self.compare(entries, hashes)

    def schema_reject(self, statement, code):
        with self.db.cursor() as cursor:
            cursor.execute('BEGIN ISOLATION LEVEL REPEATABLE READ')
            try:
                cursor.execute(statement)
                with self.assertRaisesRegex(core.SnapshotError, '^QUARANTINE_LIVE_' + code + '$'):
                    core.compare_core_in_transaction(cursor, self.entries, self.hashes)
            finally:
                cursor.execute('ROLLBACK')

    def signed(self, changes=None):
        fixture = self.approval_class('test_valid_pinned_3072_signature_and_both_operations')
        fixture.setUp()
        original = fixture.manifest['executions'][0]
        fixture.manifest['executions'] = []
        fixture.manifest['approved_count'] = 9
        for entry in self.entries:
            item = dict(original, **entry)
            item['approval_id'] = 'synthetic-approval-%d' % entry['execution_id']
            # Deliberately arbitrary signed snapshot is NOT a CORE_ONLY hash.
            item['evidence'] = dict(original['evidence'], approval_id=item['approval_id'])
            item['evidence_sha256'] = hashlib.sha256(core.canonical(item['evidence'])).hexdigest()
            fixture.manifest['executions'].append(item)
        if changes:
            changes(fixture.manifest)
        return fixture.signed()

    def signed_canonical(self, changes=None, release_sha='b'*40):
        fixture = self.approval_class('test_valid_pinned_3072_signature_and_both_operations')
        fixture.setUp()
        original = fixture.manifest['executions'][0]
        fixture.manifest['release_sha'] = release_sha
        fixture.manifest['executions'] = []
        fixture.manifest['approved_count'] = 9
        snapshots = core.sample_canonical_live_read_only(self.db, self.entries)
        for entry in self.entries:
            eid = entry['execution_id']
            snap = snapshots[eid]
            snap_sha = core.canonical_live_snapshot_sha256(snap)
            item = dict(original, **entry)
            item['release_sha'] = release_sha
            item['snapshot_sha256'] = snap_sha
            item['approval_id'] = 'synthetic-approval-%d' % eid
            evidence = dict(
                approval_id=item['approval_id'],
                snapshot_sha256=snap_sha,
                review_sha256='a'*64,
            )
            item['evidence'] = evidence
            item['evidence_sha256'] = hashlib.sha256(core.canonical(evidence)).hexdigest()
            fixture.manifest['executions'].append(item)
        if changes:
            changes(fixture.manifest)
        return fixture.signed()


    def final_reject(self, raw, code, pin=None):
        with self.db.cursor() as cursor:
            cursor.execute('BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY')
            try:
                with self.assertRaisesRegex(core.SnapshotError, '^QUARANTINE_LIVE_' + code + '$'):
                    core.validate_live_approval_in_transaction(cursor, raw, pin or hashlib.sha256(raw).hexdigest(), 'b'*40, 'enroll')
            finally:
                cursor.execute('ROLLBACK')

    def stored_asset_fixture(self, video=False):
        # Real writers: generatedAssetForRequest/insertAsset, storage service
        # preparePendingUpload/CompleteUpload, migration115 content-address claim.
        self.settle_modern_capture()
        self.sql('DELETE FROM public.provider_execution_correlations WHERE execution_id=800')
        self.sql('DELETE FROM public.provider_executions WHERE id=800')
        self.sql('UPDATE public.provider_executions SET task_execution_generation=2 WHERE id=901')
        self.entries[0]['generation']=2
        self.sql("INSERT INTO public.xz_storage_configs(id,name,provider,endpoint,bucket,is_default,access_key_encrypted,secret_key_encrypted) VALUES('cfg','synthetic','minio','https://SENSITIVE_SYNTHETIC.invalid','synthetic-bucket',true,'SENSITIVE_SYNTHETIC_ENCRYPTED_A','SENSITIVE_SYNTHETIC_ENCRYPTED_S')")
        ext='mp4' if video else 'png'
        mime='video/mp4' if video else 'image/png'
        filehash=hashlib.sha256(b'synthetic-content-not-remote').hexdigest()
        name='synthetic-task-0-01.'+ext
        digest=hashlib.sha256(('tenant_default|generation_result|synthetic-task-0|'+name+'|'+filehash).encode()).hexdigest()[:32]
        key='tenants/tenant_default/generation_result/artifacts/'+digest+'.'+ext
        record=dict(fileId='file0',tenantId='tenant_default',provider='minio',bucket='synthetic-bucket',objectKey=key,fileSize=17,contentType=mime,source='model-provider',providerTaskId='synthetic-provider-id')
        self.sql("INSERT INTO public.xz_file_objects(file_id,tenant_id,user_id,storage_config_id,provider,bucket,object_key,original_name,stored_name,extension,mime_type,file_size,file_hash,hash_algorithm,business_type,business_id,status,metadata) VALUES('file0','tenant_default','synthetic-user-0','cfg','minio','synthetic-bucket',%s,%s,%s,%s,%s,17,%s,'sha256','generation_result','synthetic-task-0','ACTIVE',%s::jsonb)", (key,name,digest+'.'+ext,ext,mime,filehash,json.dumps(dict(declaredSize=17,declaredMimeType=mime))))
        metadata=dict(index=1,prompt='SENSITIVE_SYNTHETIC_PROMPT',model='synthetic-model',type='TEXT_TO_VIDEO' if video else 'TEXT_TO_IMAGE',fileId='file0',storageFileId='file0',storageTenantId='tenant_default',storageProvider='minio',storageBucket='synthetic-bucket',storageObjectKey=key,fileSize=17,fileSizeBytes=17,contentType=mime,storageManaged=True)
        url='storage://file0' if video else 'https://SENSITIVE_SYNTHETIC.invalid/result.png'
        raw=dict(id='asset0',userId='synthetic-user-0',taskId='synthetic-task-0',name='synthetic',mediaType='video' if video else 'image',url=url,thumbnailUrl=url,metadata=metadata,favorite=False,createdAt='2026-01-01T00:00:00Z',updatedAt='2026-01-01T00:00:00Z')
        self.sql('INSERT INTO public.xz_assets(id,user_id,task_id,name,media_type,url,thumbnail_url,metadata,raw,created_at,updated_at) VALUES(%s,%s,%s,%s,%s,%s,%s,%s::jsonb,%s::jsonb,%s,%s)',tuple(raw[k] for k in ('id','userId','taskId','name','mediaType','url','thumbnailUrl'))+(json.dumps(metadata),json.dumps(raw),raw['createdAt'],raw['updatedAt']))
        result=dict(provider='synthetic-provider',status='succeeded',videoUrl=url) if video else [dict(URL=url,ThumbnailURL=url,ContentType=mime,Width=10,Height=10,Source='model-provider',ProviderTaskID='synthetic-provider-id',RevisedPrompt='',ProviderMetadata=None)]
        self.sql("UPDATE public.provider_executions SET capability=%s,status='succeeded',result_metadata=%s::jsonb WHERE id=901",('video' if video else 'image',json.dumps(result)))
        self.sql("UPDATE public.xz_generation_tasks SET type=%s,result_ids='[\"asset0\"]',params=params||jsonb_build_object('generated_storage_files',%s::jsonb),raw=raw||'{\"resultIds\":[\"asset0\"]}'::jsonb WHERE id='synthetic-task-0'",('TEXT_TO_VIDEO' if video else 'TEXT_TO_IMAGE',json.dumps([record])))
        self.asset_base()

    def asset_base(self):
        rows=core.sample_core_financial_assets_read_only(self.db,self.entries)
        self.asset_hashes={eid:core.core_financial_assets_sha256(row) for eid,row in rows.items()}
        return rows

    def asset_mutations(self,cases):
        for sql,code in cases:
            with self.subTest(sql=sql):
                with self.db.cursor() as cursor:
                    cursor.execute('BEGIN ISOLATION LEVEL REPEATABLE READ')
                    try:
                        cursor.execute(sql)
                        with self.assertRaisesRegex(core.SnapshotError,'^QUARANTINE_LIVE_'+code+'$'):
                            core.compare_core_financial_assets_in_transaction(cursor,self.entries,self.asset_hashes)
                    finally:
                        cursor.execute('ROLLBACK')

    def test_assets_writer_image_positive_privacy_and_drift(self):
        self.stored_asset_fixture()
        rows=self.asset_base()
        self.assertEqual(rows[901]['asset_storage']['counts'],dict(assets=1,files=1,configs=1,relations=0,jobs=0,multipart=0,parts=0))
        for secret in (b'SENSITIVE_SYNTHETIC',b'https://',b'storage://',b'synthetic-bucket',b'file0'):
            self.assertNotIn(secret,core.canonical(rows))
        self.asset_mutations([
            ("UPDATE public.xz_assets SET favorite=true",'ASSET_PARTIAL_HASH_MISMATCH'),
            ("UPDATE public.xz_assets SET deleted_at=now()",'ASSET_PARTIAL_HASH_MISMATCH'),
            ("UPDATE public.xz_file_objects SET etag='changed'",'ASSET_PARTIAL_HASH_MISMATCH'),
            ("UPDATE public.xz_file_objects SET metadata=metadata||'{\"remote\":\"SENSITIVE_SYNTHETIC\"}'",'ASSET_PARTIAL_HASH_MISMATCH'),
            ("UPDATE public.xz_storage_configs SET secret_key_encrypted='changed'",'ASSET_PARTIAL_HASH_MISMATCH'),
            ("UPDATE public.xz_storage_configs SET endpoint='changed'",'ASSET_PARTIAL_HASH_MISMATCH'),
            ("UPDATE public.provider_executions SET result_metadata=jsonb_set(result_metadata,'{0,URL}','\"https://changed.invalid\"') WHERE id=901",'ASSET_PARTIAL_HASH_MISMATCH'),
            ("UPDATE public.xz_generation_tasks SET raw=raw||'{\"resultUrl\":\"https://changed.invalid\"}' WHERE id='synthetic-task-0'",'ASSET_PARTIAL_HASH_MISMATCH'),
        ])

    def test_assets_writer_video_positive_and_conflicts(self):
        self.stored_asset_fixture(video=True)
        self.assertEqual(self.asset_base()[901]['asset_storage']['counts']['assets'],1)
        self.asset_mutations([
            ("UPDATE public.xz_assets SET url='storage://missing'",'RESULT_REFERENCE_MISSING'),
            ("UPDATE public.xz_assets SET metadata=jsonb_set(metadata,'{storageFileId}','\"missing\"')",'RESULT_REFERENCE_MISSING'),
            ("UPDATE public.xz_file_objects SET business_id='other'",'STORAGE_CLAIM_INVALID'),
            ("UPDATE public.xz_file_objects SET user_id='other'",'STORAGE_CLAIM_INVALID'),
            ("UPDATE public.xz_file_objects SET tenant_id='other'",'STORAGE_CLAIM_INVALID'),
            ("UPDATE public.xz_file_objects SET storage_config_id='env_default'",'UNSUPPORTED_STORAGE_CONFIG'),
            ("DELETE FROM public.xz_storage_configs",'STORAGE_CLAIM_INVALID'),
            ("DELETE FROM public.xz_file_objects",'RESULT_REFERENCE_MISSING'),
            ("DROP INDEX public.ux_file_objects_generation_artifact_identity",'STORAGE_CLAIM_SCHEMA_INVALID'),
        ])

    def test_assets_result_membership_owners_reverse_links(self):
        self.stored_asset_fixture()
        self.asset_mutations([
            ("UPDATE public.xz_generation_tasks SET result_ids='[\"asset0\",\"asset0\"]' WHERE id='synthetic-task-0'",'RESULT_STATE_INVALID'),
            ("UPDATE public.xz_generation_tasks SET result_ids='[\"missing\"]',raw=raw||'{\"resultIds\":[\"missing\"]}' WHERE id='synthetic-task-0'",'RESULT_REFERENCE_MISSING'),
            ("UPDATE public.xz_generation_tasks SET result_ids='[]',raw=raw||'{\"resultIds\":[]}' WHERE id='synthetic-task-0'",'ASSET_LINKAGE_INVALID'),
            ("UPDATE public.xz_assets SET user_id='other'",'ASSET_LINKAGE_INVALID'),
            ("UPDATE public.xz_assets SET tenant_id='other'",'ASSET_LINKAGE_INVALID'),
            ("UPDATE public.xz_assets SET task_id='other',raw=raw||'{\"taskId\":\"other\"}'",'ASSET_LINKAGE_INVALID'),
            ("UPDATE public.xz_assets SET metadata=jsonb_set(metadata,'{index}','0')",'ASSET_LINKAGE_INVALID'),
            ("UPDATE public.xz_generation_tasks SET result_ids='[\"asset0\"]' WHERE id='synthetic-task-1'",'ASSET_LINKAGE_INVALID'),
            ("UPDATE public.provider_executions SET task_execution_generation=1 WHERE id=901",'EXECUTION_GENERATION_MISMATCH'),
            ("UPDATE public.provider_executions SET task_id='other' WHERE id=901",'EXECUTION_COUNT_MISMATCH'),
            ("UPDATE public.xz_assets SET raw='{}'",'ASSET_LINKAGE_INVALID'),
            ("DELETE FROM public.xz_assets",'RESULT_REFERENCE_MISSING'),
        ])

    def test_assets_auxiliary_links_explicitly_unsupported(self):
        self.stored_asset_fixture()
        self.asset_mutations([
            ("INSERT INTO public.xz_file_relations(id,tenant_id,source_file_id,target_file_id,relation_type) VALUES('rel','tenant_default','file0','file0','unknown')",'UNSUPPORTED_STORAGE_LINK'),
            ("INSERT INTO public.xz_storage_jobs(id,tenant_id,file_id,job_type) VALUES('job','tenant_default','file0','unknown')",'UNSUPPORTED_STORAGE_LINK'),
            ("INSERT INTO public.xz_storage_jobs(id,tenant_id,job_type,metadata) VALUES('job','other','unknown','{\"nested\":{\"task\":\"synthetic-task-0\"}}')",'UNSUPPORTED_STORAGE_LINK'),
            ("INSERT INTO public.xz_multipart_uploads(id,tenant_id,owner_user_id,file_id,provider_upload_id,object_key,file_name,total_size,part_size,total_parts,state,expires_at) SELECT 'm',tenant_id,user_id,'missing','SENSITIVE_SYNTHETIC',object_key,'unrelated',17,17,1,'initialized',now() FROM public.xz_file_objects; INSERT INTO public.xz_multipart_upload_parts(upload_id,part_number,etag) VALUES('m',1,'SENSITIVE_SYNTHETIC')",'UNSUPPORTED_STORAGE_LINK'),
        ])

    def test_assets_schema_zero_vs_missing_provider_and_count(self):
        self.asset_base()
        self.asset_mutations([
            ("ALTER TABLE public.xz_assets ADD unknown text",'SCHEMA_MISMATCH'),
            ("ALTER TABLE public.xz_file_objects DROP COLUMN etag",'SCHEMA_MISMATCH'),
            ("DROP TABLE public.xz_storage_jobs",'SCHEMA_MISMATCH'),
            ("UPDATE public.provider_executions SET result_metadata='null' WHERE id=901",'PROVIDER_RESULT_STATE_INVALID'),
            ("UPDATE public.provider_executions SET status='succeeded' WHERE id=901",'PROVIDER_RESULT_STATE_INVALID'),
            ("UPDATE public.xz_generation_tasks SET raw=raw||'{\"resultIds\":null}' WHERE id='synthetic-task-0'",'RESULT_STATE_INVALID'),
            ("UPDATE public.xz_generation_tasks SET params=params||'{\"generated_storage_files\":null}' WHERE id='synthetic-task-0'",'RESULT_STATE_INVALID'),
            ("INSERT INTO public.xz_assets(id,task_id) SELECT 'overflow-'||n,'synthetic-task-0' FROM generate_series(1,10001) n",'COUNT_LIMIT'),
        ])

    def test_assets_reverse_file_refs_duplicates_and_attempt_ambiguity(self):
        self.stored_asset_fixture()
        self.asset_mutations([
            ("UPDATE public.xz_generation_tasks SET params=params||'{\"generated_storage_files\":[{\"fileId\":\"file0\"}]}' WHERE id='synthetic-task-1'",'STORAGE_REVERSE_REFERENCE_CONFLICT'),
            ("UPDATE public.xz_generation_tasks SET raw=raw||'{\"resultUrl\":\"storage://file0\"}' WHERE id='synthetic-task-1'",'STORAGE_REVERSE_REFERENCE_CONFLICT'),
            ("UPDATE public.provider_executions SET result_metadata='[{\"URL\":\"storage://file0\"}]' WHERE id=902",'STORAGE_REVERSE_REFERENCE_CONFLICT'),
            ("ALTER TABLE public.xz_assets DROP CONSTRAINT xz_assets_pkey; INSERT INTO public.xz_assets SELECT * FROM public.xz_assets",'ASSET_DUPLICATE_IDENTITY'),
            ("INSERT INTO public.provider_executions(id,task_id,attempt,status,task_execution_generation,provider,provider_channel,provider_model,capability,request_fingerprint) VALUES(998,'synthetic-task-0',2,'unknown',2,'p','c','m','image',repeat('e',64))",'ASSET_ATTEMPT_ATTRIBUTION_AMBIGUOUS'),
            ("UPDATE public.xz_generation_tasks SET params=params||'{\"tenant_id\":\"other\"}' WHERE id='synthetic-task-0'",'RESULT_STATE_INVALID'),
            ("UPDATE public.xz_assets SET user_id=NULL",'ASSET_LINKAGE_INVALID'),
            ("UPDATE public.xz_file_objects SET object_key='changed'",'STORAGE_CLAIM_INVALID'),
            ("UPDATE public.xz_storage_configs SET tenant_id='other'",'STORAGE_CLAIM_INVALID'),
            ("ALTER TABLE public.xz_file_objects ALTER COLUMN file_size TYPE numeric",'SCHEMA_MISMATCH'),
        ])

    def test_assets_unrelated_upload_excluded_and_both_relation_directions(self):
        self.stored_asset_fixture()
        self.sql("INSERT INTO public.xz_file_objects(file_id,tenant_id,user_id,storage_config_id,provider,bucket,object_key,original_name,stored_name) VALUES('upload','other','other','other','minio','other','unrelated','unrelated','unrelated')")
        self.sql("INSERT INTO public.xz_storage_jobs(id,tenant_id,file_id,job_type) VALUES('unrelated','other','upload','unknown')")
        self.assertEqual(self.asset_base()[901]['asset_storage']['counts']['files'],1)
        self.asset_mutations([
            ("INSERT INTO public.xz_file_relations(id,tenant_id,source_file_id,target_file_id,relation_type) VALUES('rel','other','upload','file0','unknown')",'UNSUPPORTED_STORAGE_LINK'),
            ("INSERT INTO public.xz_file_relations(id,tenant_id,source_file_id,target_file_id,relation_type) VALUES('rel','other','file0','upload','unknown')",'UNSUPPORTED_STORAGE_LINK'),
        ])

    def test_assets_pending_claim_and_provider_result_without_asset_positive(self):
        self.stored_asset_fixture()
        self.sql('DELETE FROM public.xz_assets')
        self.sql("UPDATE public.xz_generation_tasks SET result_ids='[]',raw=raw||'{\"resultIds\":[]}',params=params-'generated_storage_files' WHERE id='synthetic-task-0'")
        self.sql("UPDATE public.xz_file_objects SET status='PENDING_UPLOAD',file_size=0,reserved_size=17")
        rows=self.asset_base()
        self.assertEqual(rows[901]['asset_storage']['counts']['assets'],0)
        self.assertEqual(rows[901]['asset_storage']['counts']['files'],1)
        self.sql('DELETE FROM public.xz_file_objects')
        self.assertEqual(self.asset_base()[901]['asset_storage']['counts']['files'],0)
        # Existing provider success remains bound in core; zero local rows never
        # means provider failure, no late success, or safe activation.

    def test_assets_empty_known_db_path(self):
        rows=core.sample_core_financial_assets_read_only(self.db,self.entries)
        self.assertEqual(len(rows),9)
        self.assertEqual(rows[901]['asset_storage']['counts'], dict(assets=0,files=0,configs=0,relations=0,jobs=0,multipart=0,parts=0))

    def test_internal_nine_row_stable_positive_and_privacy(self):
        snapshots = self.compare()
        self.assertEqual(len(snapshots), 9)
        self.assertEqual(snapshots[901]['counts'], dict(task=1, attempts=2, correlations=2))
        self.assertEqual(snapshots[902]['counts'], dict(task=1, attempts=1, correlations=1))
        self.sql('SELECT pg_sleep(0.01)')  # DB observation clock changes, stable hash must not.
        self.assertEqual(self.hashes, {eid: core.core_sha256(value) for eid, value in core.sample_core_read_only(self.db, list(reversed(self.entries))).items()})
        encoded = core.canonical(snapshots)
        for sensitive in (b'SENSITIVE_SYNTHETIC', b'https://', b'possibly_submitted'):
            self.assertNotIn(sensitive, encoded)
        with self.db.cursor() as cursor:
            cursor.execute('SELECT count(*) FROM public.xz_personal_point_reservations')
            self.assertEqual(cursor.fetchone()[0], 6)  # not all9 have personal reservations

    def test_both_live_generations_and_prior_generation999(self):
        for field, code in [('generation', 'EXECUTION_GENERATION_MISMATCH'), ('task_generation', 'TASK_GENERATION_MISMATCH')]:
            changed = copy.deepcopy(self.entries)
            changed[0][field] = 999
            self.reject(code, changed)
            self.final_reject(self.signed(lambda manifest: manifest['executions'][0].update({field: 999})), code)
        self.sql('UPDATE public.provider_executions SET task_execution_generation=7 WHERE id=901')
        self.reject('EXECUTION_GENERATION_MISMATCH')

    def test_task_live_generation_drift(self):
        self.sql('UPDATE public.xz_generation_tasks SET execution_generation=7 WHERE id=%s', ('synthetic-task-0',))
        self.reject('TASK_GENERATION_MISMATCH')

    def test_arbitrary_core_hash_and_missing_hash_counts(self):
        hashes = dict(self.hashes)
        hashes[901] = '0'*64
        self.reject('CORE_HASH_MISMATCH', hashes=hashes)
        del hashes[901]
        self.reject('APPROVED_COUNT_INVALID', hashes=hashes)

    def test_authenticated_arbitrary_final_hash_is_never_accepted(self):
        self.final_reject(self.signed(), 'SNAPSHOT_SHA256_MISMATCH')

    def test_identity_attempt_and_duplicate_approved_counts(self):
        for key, value, code in [('execution_id', 700, 'EXECUTION_IDENTITY_MISMATCH'), ('task_id', 'missing-task', 'TASK_COUNT_MISMATCH'), ('attempt', 5, 'ATTEMPT_MISMATCH')]:
            changed = copy.deepcopy(self.entries)
            changed[0][key] = value
            self.reject(code, changed)
        self.reject('DUPLICATE_APPROVED_IDENTITY', self.entries + [self.entries[0]])
        self.reject('APPROVED_COUNT_INVALID', [])

    def test_execution_task_status_provider_owner_lease_payload_drifts(self):
        cases = [
            ('provider_executions', 'status', 'processing'),
            ('provider_executions', 'provider', 'other'),
            ('provider_executions', 'provider_channel', 'other'),
            ('provider_executions', 'provider_model', 'other'),
            ('provider_executions', 'provider_request_id', 'other'),
            ('provider_executions', 'provider_operation_key', 'other'),
            ('provider_executions', 'request_fingerprint', 'c'*64),
            ('provider_executions', 'result_metadata', '{"url":"https://changed.invalid"}'),
            ('provider_executions', 'last_error', 'changed-private-error'),
            ('xz_generation_tasks', 'status', 'PROCESSING'),
            ('xz_generation_tasks', 'task_status', 'RUNNING'),
            ('xz_generation_tasks', 'worker_id', 'other-owner'),
            ('xz_generation_tasks', 'lease_until', '2000-01-01T00:00:00Z'),
            ('xz_generation_tasks', 'last_heartbeat_at', '2000-01-01T00:00:00Z'),
            ('xz_generation_tasks', 'prompt', 'changed-private-prompt'),
            ('xz_generation_tasks', 'params', '{"url":"https://changed.invalid"}'),
            ('xz_generation_tasks', 'result_ids', '["missing-synthetic-result"]'),
            ('xz_generation_tasks', 'reserved_points', 6),
            ('xz_generation_tasks', 'client_request_id', 'other-client'),
            ('xz_generation_tasks', 'user_id', 'other-user'),
        ]
        for table, column, value in cases:
            with self.subTest(table=table, column=column):
                # Static identifiers from this fixture, never untrusted input.
                with self.db.cursor() as cursor:
                    cursor.execute('BEGIN ISOLATION LEVEL REPEATABLE READ')
                    try:
                        cursor.execute('UPDATE public.' + table + ' SET ' + column + '=%s WHERE id=%s', (value, 901 if table == 'provider_executions' else 'synthetic-task-0'))
                        with self.assertRaisesRegex(core.SnapshotError, '^QUARANTINE_LIVE_CORE_HASH_MISMATCH$'):
                            core.compare_core_in_transaction(cursor, self.entries, self.hashes)
                    finally:
                        cursor.execute('ROLLBACK')

    def test_prior_attempt_and_correlation_drift(self):
        self.sql("UPDATE public.provider_execution_correlations SET provider_state='changed' WHERE id=1800")
        self.reject('CORE_HASH_MISMATCH')

    def test_correlation_delete_and_insert_count_drift(self):
        self.sql('DELETE FROM public.provider_execution_correlations WHERE id=1901')
        self.reject('CORE_HASH_MISMATCH')
        self.sql("INSERT INTO public.provider_execution_correlations(id,execution_id,kind) VALUES(2000,901,'other')")
        self.reject('CORE_HASH_MISMATCH')

    def test_correlation_single_field_drift_and_nullable_provider_request(self):
        cases = [('kind', 'changed'), ('provider_code', 'changed'), ('base_url_host', 'changed'),
                 ('endpoint_path', '/changed'), ('provider_job_id', 'changed'), ('job_role', 'changed'),
                 ('provider_state', 'changed'), ('http_status', 202), ('error_code', 'changed'),
                 ('error_hash', 'e'*64), ('created_at', '2000-01-01T00:00:00Z')]
        for column, value in cases:
            with self.subTest(column=column):
                with self.db.cursor() as cursor:
                    cursor.execute('BEGIN ISOLATION LEVEL REPEATABLE READ')
                    try:
                        cursor.execute('UPDATE public.provider_execution_correlations SET ' + column + '=%s WHERE id=1901', (value,))
                        with self.assertRaisesRegex(core.SnapshotError, 'CORE_HASH_MISMATCH'):
                            core.compare_core_in_transaction(cursor, self.entries, self.hashes)
                    finally:
                        cursor.execute('ROLLBACK')
        self.sql('UPDATE public.provider_executions SET provider_request_id=NULL WHERE id=901')
        self.reject('CORE_HASH_MISMATCH')

    def test_missing_task_and_excessive_attempt_count(self):
        self.schema_reject("DELETE FROM public.xz_generation_tasks WHERE id='synthetic-task-0'", 'TASK_COUNT_MISMATCH')
        self.sql("INSERT INTO public.provider_executions(id,task_id,attempt,status,task_execution_generation,provider,provider_channel,provider_model,capability,request_fingerprint) SELECT 50000+n,'synthetic-task-0',10+n,'failed',1,'synthetic-provider','synthetic-channel','synthetic-model','image',repeat('b',64) FROM generate_series(1,10000) n")
        self.reject('COUNT_LIMIT')

    def test_prior_attempt_deletion_count_drift(self):
        self.sql('DELETE FROM public.provider_executions WHERE id=800')
        self.reject('CORE_HASH_MISMATCH')

    def test_missing_selected_execution(self):
        self.sql('DELETE FROM public.provider_executions WHERE id=901')
        self.reject('EXECUTION_IDENTITY_MISMATCH')

    def test_null_required_generation_status_identity_and_channel(self):
        for table, column, value, code in [
            ('provider_executions', 'task_execution_generation', None, 'NULL_OR_UNKNOWN_EXECUTION_STATE'),
            ('provider_executions', 'provider_channel', '', 'NULL_OR_UNKNOWN_EXECUTION_STATE'),
            ('xz_generation_tasks', 'status', None, 'NULL_OR_UNKNOWN_TASK_STATE'),
            ('xz_generation_tasks', 'user_id', None, 'NULL_OR_UNKNOWN_TASK_STATE'),
        ]:
            with self.subTest(column=column):
                with self.db.cursor() as cursor:
                    cursor.execute('BEGIN ISOLATION LEVEL REPEATABLE READ')
                    try:
                        cursor.execute('UPDATE public.' + table + ' SET ' + column + '=%s WHERE id=%s', (value, 901 if table == 'provider_executions' else 'synthetic-task-0'))
                        with self.assertRaisesRegex(core.SnapshotError, '^QUARANTINE_LIVE_' + code + '$'):
                            core.project_core_in_transaction(cursor, self.entries)
                    finally:
                        cursor.execute('ROLLBACK')

    def test_missing_changed_extra_schema_columns(self):
        self.schema_reject('ALTER TABLE public.xz_generation_tasks DROP COLUMN prompt', 'SCHEMA_MISMATCH')
        self.schema_reject('ALTER TABLE public.provider_executions ADD COLUMN unreviewed text', 'SCHEMA_MISMATCH')
        self.schema_reject('ALTER TABLE public.xz_generation_tasks ALTER COLUMN progress TYPE bigint', 'SCHEMA_MISMATCH')
        self.schema_reject('ALTER TABLE public.provider_execution_correlations RENAME TO missing_table', 'SCHEMA_MISMATCH')
        self.schema_reject('ALTER TABLE public.xz_generation_tasks ENABLE ROW LEVEL SECURITY', 'SCHEMA_MISMATCH')

    def test_duplicate_execution_attempt_rows(self):
        self.schema_reject("ALTER TABLE public.provider_executions DROP CONSTRAINT provider_executions_task_attempt_unique; INSERT INTO public.provider_executions SELECT (jsonb_populate_record(NULL::public.provider_executions,to_jsonb(e)||'{\"id\":7000}'::jsonb)).* FROM public.provider_executions e WHERE id=800", 'DUPLICATE_EXECUTION_IDENTITY')

    def test_duplicate_execution_id_on_other_task(self):
        self.schema_reject("ALTER TABLE public.provider_executions DROP CONSTRAINT provider_executions_pkey CASCADE; INSERT INTO public.provider_executions SELECT (jsonb_populate_record(NULL::public.provider_executions,to_jsonb(e)||'{\"task_id\":\"synthetic-task-1\",\"attempt\":3,\"status\":\"failed\"}'::jsonb)).* FROM public.provider_executions e WHERE id=901", 'DUPLICATE_EXECUTION_IDENTITY')

    def test_duplicate_correlation_id_rows(self):
        self.schema_reject('ALTER TABLE public.provider_execution_correlations DROP CONSTRAINT provider_execution_correlations_pkey; INSERT INTO public.provider_execution_correlations SELECT * FROM public.provider_execution_correlations WHERE id=1901', 'CORRELATION_IDENTITY_INVALID')

    def test_duplicate_task_ambiguous_count(self):
        self.schema_reject("ALTER TABLE public.xz_generation_tasks DROP CONSTRAINT xz_generation_tasks_pkey; INSERT INTO public.xz_generation_tasks SELECT * FROM public.xz_generation_tasks WHERE id='synthetic-task-0'", 'TASK_COUNT_MISMATCH')

    def test_db_function_failure_is_redacted_not_empty_safe(self):
        self.schema_reject('DROP EXTENSION pgcrypto CASCADE', 'DB_QUERY_FAILED')

    def test_read_only_and_coherent_snapshot_under_concurrent_change(self):
        import psycopg2
        writer = psycopg2.connect(host='127.0.0.1', dbname='xianzhi_test', user='postgres', password='test_secret_pass')
        writer.autocommit = True
        try:
            with self.db.cursor() as cursor:
                cursor.execute('BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY')
                try:
                    first = core.project_core_in_transaction(cursor, self.entries)
                    with writer.cursor() as other:
                        other.execute("UPDATE public.provider_executions SET provider_request_id='concurrent' WHERE id=901")
                    second = core.project_core_in_transaction(cursor, self.entries)
                    self.assertEqual(first, second)
                    cursor.execute("SHOW transaction_read_only")
                    self.assertEqual(cursor.fetchone(), ('on',))
                    with self.assertRaises(Exception):
                        cursor.execute("UPDATE public.xz_generation_tasks SET prompt='forbidden'")
                finally:
                    cursor.execute('ROLLBACK')
            self.reject('CORE_HASH_MISMATCH')
        finally:
            writer.close()

    def test_hostile_task_ids_are_parameters_never_structure(self):
        for value in ["single'quote", 'double"quote', '$(touch /tmp/ISSUE203_LIVE_PWNED)', '`touch /tmp/ISSUE203_LIVE_PWNED`', '; DROP TABLE provider_executions;', 'line1\nline2', '中文', 'json\\escape']:
            with self.subTest(value=value):
                with self.db.cursor() as cursor:
                    cursor.execute('BEGIN ISOLATION LEVEL REPEATABLE READ')
                    try:
                        cursor.execute('UPDATE public.xz_generation_tasks SET id=%s WHERE id=%s', (value, 'synthetic-task-0'))
                        cursor.execute('UPDATE public.provider_executions SET task_id=%s WHERE task_id=%s', (value, 'synthetic-task-0'))
                        entries = copy.deepcopy(self.entries)
                        entries[0]['task_id'] = value
                        self.assertEqual(len(core.project_core_in_transaction(cursor, entries)), 9)
                    finally:
                        cursor.execute('ROLLBACK')
        self.assertFalse(Path('/tmp/ISSUE203_LIVE_PWNED').exists())
        changed = copy.deepcopy(self.entries)
        changed[0]['task_id'] = 'x'*257
        self.reject('IDENTITY_INVALID', changed)

    def test_unsupported_enterprise_uuid_and_unknown_linkage(self):
        changed = copy.deepcopy(self.entries)
        changed[0]['task_id'] = str(uuid.uuid4())
        self.reject('UNSUPPORTED_LINKAGE', changed)
        self.sql("UPDATE public.xz_generation_tasks SET billing_account_type='ENTERPRISE' WHERE id='synthetic-task-0'")
        self.reject('UNSUPPORTED_LINKAGE')

    def test_params_billing_scope_rejected_at_core_projection(self):
        cases = [
            ('PERSONAL', {'billing_scope': 'ENTERPRISE'}),
            ('PERSONAL', {'billing_scope': ' eNtErPrIsE\t'}),
            ('PERSONAL', {'billing_scope': '\u0085\u2003ENTERPRISE\u3000'}),
            ('PERSONAL', {'billing_scope': 'UNKNOWN'}),
            ('PERSONAL', {'billing_scope': 'PERSONAL\u200b'}),
            ('PERSONAL', {'billing_scope': None}),
            ('PERSONAL', {'billing_scope': 1}),
            ('PERSONAL', {'billing_scope': True}),
            ('PERSONAL', {'billing_scope': []}),
            ('PERSONAL', {'billing_scope': {}}),
            ('PERSONAL', []), ('PERSONAL', None), ('PERSONAL', 'PERSONAL'),
            ('PERSONAL', 1), ('PERSONAL', True),
            ('ENTERPRISE', {'billing_scope': 'PERSONAL'}),
            (' unknown ', {'billing_scope': 'PERSONAL'}),
            ('', {'billing_scope': 'ENTERPRISE'}),
        ]
        for account, params in cases:
            with self.subTest(account=account, params_type=type(params).__name__):
                self.sql('UPDATE public.xz_generation_tasks SET billing_account_type=%s,params=%s::jsonb WHERE id=%s',
                         (account, json.dumps(params), 'synthetic-task-0'))
                with self.assertRaisesRegex(core.SnapshotError, '^QUARANTINE_LIVE_UNSUPPORTED_LINKAGE$'):
                    core.sample_core_read_only(self.db, self.entries)

    def test_params_billing_scope_personal_and_missing_controls(self):
        for account, params in [
            ('PERSONAL', {}), ('PERSONAL', {'billing_scope': 'PERSONAL'}),
            ('PERSONAL', {'billing_scope': ' pErSoNaL\t\n'}),
            ('PERSONAL', {'billing_scope': '\u0085\u2003personal\u3000'}),
            ('PERSONAL', {'billing_scope': ' \t'}),
            ('', {}), ('', {'billing_scope': 'PERSONAL'}),
            (' personal\t', {'billing_scope': ''}),
        ]:
            with self.subTest(account=account):
                self.sql('UPDATE public.xz_generation_tasks SET billing_account_type=%s,params=%s::jsonb WHERE id=%s',
                         (account, json.dumps(params), 'synthetic-task-0'))
                snapshots = core.sample_core_read_only(self.db, self.entries)
                self.assertEqual(len(snapshots), 9)
                hashes = {eid: core.core_sha256(value) for eid, value in snapshots.items()}
                self.assertEqual(self.compare(hashes=hashes), snapshots)

    def test_exact_file_bytes_db_window_and_signature_tampering(self):
        raw = self.signed()
        changed = json.dumps(json.loads(raw.decode())).encode()
        self.final_reject(changed, 'MANIFEST_BYTES_MISMATCH', hashlib.sha256(raw).hexdigest())
        self.final_reject(raw, 'MANIFEST_BYTES_MISMATCH', '0'*64)
        manifest = json.loads(raw.decode())
        manifest['signature'] = 'AA=='
        self.final_reject(core.canonical(manifest), 'APPROVAL_INVALID')
        expired = self.signed(lambda manifest: manifest.update(expires_at='2000-01-01T00:00:00.000000Z'))
        self.final_reject(expired, 'APPROVAL_INVALID')
        future = self.signed(lambda manifest: manifest.update(not_before='2100-01-01T00:00:00.000000Z'))
        self.final_reject(future, 'APPROVAL_INVALID')
        self.final_reject(self.signed(lambda manifest: manifest.update(expires_at=None)), 'APPROVAL_INVALID')
        self.final_reject(self.signed(lambda manifest: manifest.update(approved=True)), 'APPROVAL_INVALID')
        self.final_reject(self.signed(lambda manifest: manifest.update(approved_count=8)), 'APPROVAL_INVALID')

    def test_transaction_and_disconnected_db_fail_closed(self):
        with self.db.cursor() as cursor:
            with self.assertRaisesRegex(core.SnapshotError, 'TRANSACTION_REQUIRED'):
                core.project_core_in_transaction(cursor, self.entries)
        import psycopg2
        disconnected = psycopg2.connect(host='127.0.0.1', dbname='xianzhi_test', user='postgres', password='test_secret_pass')
        disconnected.autocommit = True
        disconnected.close()
        with self.assertRaisesRegex(core.SnapshotError, 'DB_QUERY_FAILED'):
            core.sample_core_read_only(disconnected, self.entries)

    def financial_base(self):
        snapshots = core.sample_core_financial_read_only(self.db, self.entries)
        self.partial_hashes = {eid: core.core_financial_sha256(value) for eid,value in snapshots.items()}
        return snapshots

    def financial_compare(self, cursor=None):
        if cursor is not None:
            return core.compare_core_financial_in_transaction(cursor, self.entries, self.partial_hashes)
        with self.db.cursor() as cursor:
            cursor.execute('BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY')
            try:
                return self.financial_compare(cursor)
            finally:
                cursor.execute('ROLLBACK')

    def financial_reject(self, code):
        with self.assertRaisesRegex(core.SnapshotError, '^QUARANTINE_LIVE_' + code + '$'):
            self.financial_compare()

    def settle_modern_capture(self):
        # CreateGenerationTask synchronous writer: no legacy reservation params.
        self.sql("UPDATE public.xz_generation_tasks SET params=params-'billingReserved'-'billingReservationPointCost',status='SUCCEEDED',task_status='SUCCEEDED',billing_status='CAPTURED',captured_points=5 WHERE id='synthetic-task-0'")
        self.sql("UPDATE public.xz_point_accounts SET frozen=0 WHERE id='synthetic-account-0'")
        self.sql("UPDATE public.xz_user_wallets SET frozen_token=0,total_token_used=5 WHERE user_id='synthetic-user-0'")
        self.sql("UPDATE public.xz_personal_point_lots SET reserved_points=0,consumed_points=5 WHERE id='synthetic-lot-0'")
        for table, identity in [('xz_personal_point_reservations','synthetic-reservation-0'),('xz_personal_point_reservation_allocations','synthetic-allocation-0')]:
            self.sql('UPDATE public.'+table+" SET reserved_points=0,captured_points=5,status='CAPTURED' WHERE id=%s", (identity,))
        self.sql("INSERT INTO public.xz_wallet_ledger(id,account_id,user_id,task_id,reference_type,reference_id,entry_type,points,available_before,available_after,frozen_before,frozen_after,idempotency_key) VALUES('captured','synthetic-account-0','synthetic-user-0','synthetic-task-0','GENERATION_TASK','synthetic-task-0','CAPTURE',5,100,100,5,0,'personal-point:capture:synthetic-account-0:generation:capture:synthetic-task-0')")
        self.sql("INSERT INTO public.xz_personal_point_lot_movements(id,lot_id,account_id,user_id,movement_type,points,available_before,available_after,reserved_before,reserved_after,consumed_before,consumed_after,expired_before,expired_after,reversed_before,reversed_after,reservation_id,idempotency_key) VALUES('captured','synthetic-lot-0','synthetic-account-0','synthetic-user-0','CAPTURE',5,100,100,5,0,0,5,0,0,0,0,'synthetic-reservation-0','capture:generation:capture:synthetic-task-0:synthetic-lot-0')")
        self.sql("INSERT INTO public.xz_billing_lifecycle_events(id,task_id,user_id,event_type,billing_status,points,idempotency_key) VALUES('captured','synthetic-task-0','synthetic-user-0','CAPTURE','CAPTURED',5,'synthetic-task-0:CAPTURE')")
        self.insert_billing_history()

    def insert_billing_history(self):
        # insertBillingEvent's migration088 column shape; raw is its JSON mirror.
        event = dict(id='synthetic-history', transactionId='synthetic-transaction', userId='synthetic-user-0',
                     taskId='synthetic-task-0', moduleCode='image_generation', metricCode='image_generate',
                     quantity=1, unitAmountCents=1, amountCents=5, pointCost=5, balanceBefore=105,
                     balanceAfter=100, model='synthetic-model', status='SUCCEEDED',
                     occurredAt='2026-01-01T00:00:00Z', metadata={'source':'generation_task','prompt':'SENSITIVE_SYNTHETIC_HISTORY'})
        self.sql("INSERT INTO public.xz_billing_events(id,transaction_id,user_id,task_id,module_code,metric_code,quantity,unit_amount_cents,amount_cents,point_cost,balance_before,balance_after,model,status,occurred_at,metadata,raw) VALUES(%s,%s,%s,%s,%s,%s,1,1,5,5,105,100,%s,%s,%s,%s::jsonb,%s::jsonb)",
                 tuple(event[k] for k in ('id','transactionId','userId','taskId','moduleCode','metricCode','model','status','occurredAt'))+(json.dumps(event['metadata']),json.dumps(event)))

    def test_review_personal_authorization_is_user_not_point_account(self):
        self.sql("UPDATE public.xz_generation_tasks SET billing_account_id=user_id,params=params||jsonb_build_object('billing_account_id',user_id),raw=raw||jsonb_build_object('billingAccountId',user_id)")
        self.assertEqual(len(core.sample_core_financial_read_only(self.db,self.entries)),9)

    def test_review_modern_synchronous_capture_without_legacy_params(self):
        self.settle_modern_capture()
        snapshots=core.sample_core_financial_read_only(self.db,self.entries)
        self.assertEqual(snapshots[901]['financial']['counts']['ledger'],2)

    def test_review_matching_wallet_account_without_lot_backing(self):
        self.sql("UPDATE public.xz_point_accounts SET available=101 WHERE id='synthetic-account-0'")
        self.sql("UPDATE public.xz_user_wallets SET token_balance=101 WHERE user_id='synthetic-user-0'")
        with self.assertRaisesRegex(core.SnapshotError,'^QUARANTINE_LIVE_FINANCIAL_LOT_BALANCE_CONFLICT$'):
            core.sample_core_financial_read_only(self.db,self.entries)

    def test_review_billing_history_is_projected(self):
        self.financial_base()
        self.insert_billing_history()
        self.financial_reject('PARTIAL_HASH_MISMATCH')

    def review_cases(self, cases):
        for statement, code in cases:
            with self.subTest(code=code, statement=statement[:100]):
                with self.db.cursor() as cursor:
                    cursor.execute('BEGIN ISOLATION LEVEL REPEATABLE READ')
                    try:
                        cursor.execute(statement)
                        with self.assertRaisesRegex(core.SnapshotError, '^QUARANTINE_LIVE_'+code+'$'):
                            core.project_core_financial_in_transaction(cursor,self.entries)
                    finally:
                        cursor.execute('ROLLBACK')

    def test_review_imported_historical_settled_wallet_only(self):
        # Importer wraps every legacy entry, but attributes only active tasks.
        # These terminal tasks retain a LEGACY lot/OPENING, no modern markers.
        self.sql("UPDATE public.xz_wallet_ledger SET metadata=jsonb_build_object('legacy_ledger_id','  original-'||id||'  '),idempotency_key='personal-point:legacy-wallet:'||account_id||':'||idempotency_key||':original-'||id WHERE task_id='synthetic-task-6'")
        snapshots=self.financial_base()
        self.assertEqual(snapshots[907]['financial']['path'],'LEGACY_WALLET_ONLY')
        self.assertEqual(snapshots[907]['financial']['counts']['ledger'],2)
        self.assertEqual(snapshots[907]['financial']['counts']['reservations'],0)
        self.assertEqual(snapshots[907]['financial']['counts']['movements'],1)
        # Original idempotency key with no original ID, and fallback ID:ID,
        # are the other exact legacyWalletLedgerKey branches.
        for key,metadata in [
            ('personal-point:legacy-wallet:synthetic-account-6:synthetic-task-6:RESERVE',{}),
            ('personal-point:legacy-wallet:synthetic-account-6:original:original',{'legacy_ledger_id':'original'})]:
            self.sql("UPDATE public.xz_wallet_ledger SET idempotency_key=%s,metadata=%s::jsonb WHERE id='synthetic-ledger-6'",(key,json.dumps(metadata)))
            self.assertEqual(len(core.sample_core_financial_read_only(self.db,self.entries)),9)
        self.review_cases([
            ("UPDATE public.xz_wallet_ledger SET metadata='{}' WHERE id='synthetic-ledger-6'",'FINANCIAL_LEDGER_KEY_INVALID'),
            ("UPDATE public.xz_wallet_ledger SET metadata='{\"legacy_ledger_id\":3}' WHERE id='synthetic-ledger-6'",'FINANCIAL_LEDGER_KEY_INVALID'),
            ("UPDATE public.xz_wallet_ledger SET metadata='{\"legacy_ledger_id\":\"other\"}' WHERE id='synthetic-ledger-6'",'FINANCIAL_LEDGER_KEY_INVALID'),
            ("UPDATE public.xz_wallet_ledger SET metadata='{\"legacy_ledger_id\":\"\"}' WHERE id='synthetic-ledger-6'",'FINANCIAL_LEDGER_KEY_INVALID'),
            ("UPDATE public.xz_wallet_ledger SET idempotency_key='personal-point:legacy-wallet:wrong:original:original' WHERE id='synthetic-ledger-6'",'FINANCIAL_LEDGER_KEY_INVALID'),
            ("UPDATE public.xz_wallet_ledger SET idempotency_key='synthetic-task-6:RESERVE' WHERE id='synthetic-ledger-6'",'FINANCIAL_LEDGER_KEY_INVALID'),
            ("UPDATE public.xz_wallet_ledger SET user_id='other' WHERE id='synthetic-ledger-6'",'FINANCIAL_LEDGER_LINKAGE_INVALID')])

    def test_review_active_settled_cost_and_authorization_conflicts(self):
        self.review_cases([
            ("UPDATE public.xz_generation_tasks SET billing_account_id='synthetic-account-0' WHERE id='synthetic-task-0'",'FINANCIAL_ACCOUNT_LINKAGE_INVALID'),
            ("UPDATE public.xz_generation_tasks SET params=jsonb_set(params,'{billing_account_id}','\"synthetic-account-0\"') WHERE id='synthetic-task-0'",'FINANCIAL_ACCOUNT_LINKAGE_INVALID'),
            ("UPDATE public.xz_generation_tasks SET raw=jsonb_set(raw,'{billingAccountId}','\"synthetic-account-0\"') WHERE id='synthetic-task-0'",'FINANCIAL_ACCOUNT_LINKAGE_INVALID'),
            ("UPDATE public.xz_generation_tasks SET raw=jsonb_set(raw,'{billingAccountId}','3') WHERE id='synthetic-task-0'",'FINANCIAL_ACCOUNT_LINKAGE_INVALID'),
            ("UPDATE public.xz_generation_tasks SET params=params-'billingReserved'-'billingReservationPointCost' WHERE id='synthetic-task-0'",'FINANCIAL_RESERVE_CONFLICT'),
            ("UPDATE public.xz_generation_tasks SET params=params||'{\"billingRefunded\":true}' WHERE id='synthetic-task-0'",'FINANCIAL_RESERVE_CONFLICT'),
            ("UPDATE public.xz_generation_tasks SET released_points=4 WHERE id='synthetic-task-6'",'FINANCIAL_RESERVE_CONFLICT')])
        self.settle_modern_capture()
        self.review_cases([
            ("UPDATE public.xz_generation_tasks SET params=params||'{\"billingReservationPointCost\":4}' WHERE id='synthetic-task-0'",'FINANCIAL_RESERVE_CONFLICT'),
            ("UPDATE public.xz_generation_tasks SET params=params||'{\"billingReserved\":false}' WHERE id='synthetic-task-0'",'FINANCIAL_RESERVE_CONFLICT'),
            ("UPDATE public.xz_generation_tasks SET captured_points=4 WHERE id='synthetic-task-0'",'FINANCIAL_RESERVE_CONFLICT'),
            ("UPDATE public.xz_generation_tasks SET point_cost=4 WHERE id='synthetic-task-0'",'FINANCIAL_RESERVE_CONFLICT'),
            ("DELETE FROM public.xz_wallet_ledger WHERE id='captured'",'FINANCIAL_RESERVE_CONFLICT')])

    def test_review_billing_history_identity_deletion_and_duplicates(self):
        self.insert_billing_history()
        self.financial_base()
        self.review_cases([
            ("UPDATE public.xz_billing_events SET user_id='wrong' WHERE id='synthetic-history'",'FINANCIAL_HISTORY_LINKAGE_INVALID'),
            ("UPDATE public.xz_billing_events SET task_id='wrong' WHERE id='synthetic-history'",'FINANCIAL_HISTORY_LINKAGE_INVALID'),
            ("UPDATE public.xz_billing_events SET raw=jsonb_set(raw,'{taskId}','\"wrong\"') WHERE id='synthetic-history'",'FINANCIAL_HISTORY_LINKAGE_INVALID'),
            ("ALTER TABLE public.xz_billing_events DROP CONSTRAINT xz_billing_events_pkey; INSERT INTO public.xz_billing_events SELECT * FROM public.xz_billing_events WHERE id='synthetic-history'",'FINANCIAL_DUPLICATE_IDENTITY'),
            ("INSERT INTO public.xz_billing_events SELECT (jsonb_populate_record(NULL::public.xz_billing_events,to_jsonb(e)||jsonb_build_object('id','duplicate','raw',jsonb_set(raw,'{id}','\"duplicate\"')))).* FROM public.xz_billing_events e WHERE id='synthetic-history'",'FINANCIAL_DUPLICATE_KEY')])
        self.sql("DELETE FROM public.xz_billing_events WHERE id='synthetic-history'")
        self.financial_reject('PARTIAL_HASH_MISMATCH')

    def test_review_large_cardinality_is_bounded_at_real_cursor(self):
        class ObservedCursor:
            def __init__(self,cursor):
                self.cursor=cursor
                self.connection=cursor.connection
                self.reads=[]
            def execute(self,statement,parameters=()):
                self.statement=statement
                return self.cursor.execute(statement,parameters)
            def fetchall(self):
                rows=self.cursor.fetchall()
                self.reads.append((self.statement,len(rows)))
                return rows
        cases=[
            ("INSERT INTO public.xz_point_accounts(id,user_id) SELECT 'extra-'||n,'synthetic-user-0' FROM generate_series(1,10001) n",'FINANCIAL_ACCOUNT_COUNT',2,'xz_point_accounts'),
            ("ALTER TABLE public.xz_user_wallets DROP CONSTRAINT xz_user_wallets_pkey; INSERT INTO public.xz_user_wallets(user_id) SELECT 'synthetic-user-0' FROM generate_series(1,10001)",'FINANCIAL_WALLET_COUNT',2,'xz_user_wallets'),
            ("INSERT INTO public.xz_personal_point_reservations(id,account_id,user_id,business_type,business_id,requested_points,reserved_points,idempotency_key) SELECT 'extra-'||n,'synthetic-account-0','synthetic-user-0','OTHER-'||n,'synthetic-task-0',5,5,'extra-'||n FROM generate_series(1,10001) n",'FINANCIAL_RESERVATION_COUNT',2,'xz_personal_point_reservations'),
            ("INSERT INTO public.xz_billing_lifecycle_events(id,task_id,user_id,event_type,billing_status,idempotency_key) SELECT 'extra-'||n,'synthetic-task-0','synthetic-user-0','QUOTE','QUOTED','extra-'||n FROM generate_series(1,10001) n",'COUNT_LIMIT',10001,'xz_billing_lifecycle_events'),
            ("INSERT INTO public.xz_billing_events(id,task_id,user_id) SELECT 'extra-'||n,'synthetic-task-0','synthetic-user-0' FROM generate_series(1,10001) n",'COUNT_LIMIT',10001,'xz_billing_events'),
            ("INSERT INTO public.xz_wallet_ledger(id,account_id,user_id,task_id,entry_type,points,available_before,available_after,frozen_before,frozen_after,idempotency_key) SELECT 'extra-'||n,'synthetic-account-0','synthetic-user-0','synthetic-task-0','GRANT',1,0,1,0,0,'extra-'||n FROM generate_series(1,10001) n",'COUNT_LIMIT',10001,'xz_wallet_ledger')]
        for statement,code,bound,table in cases:
            with self.subTest(table=table):
                with self.db.cursor() as cursor:
                    cursor.execute('BEGIN ISOLATION LEVEL REPEATABLE READ')
                    try:
                        cursor.execute(statement)
                        observed=ObservedCursor(cursor)
                        with self.assertRaisesRegex(core.SnapshotError,'^QUARANTINE_LIVE_'+code+'$'):
                            core.project_core_financial_in_transaction(observed,self.entries[:1])
                        sql,count=observed.reads[-1]
                        self.assertIn(table,sql)
                        self.assertTrue(sql.endswith(' LIMIT '+str(bound)),sql)
                        self.assertEqual(count,bound)
                        self.assertLessEqual(max(row[1] for row in observed.reads),10001)
                    finally:
                        cursor.execute('ROLLBACK')
        print('FINANCIAL_CARDINALITY_CASES=6 actual cursor LIMIT2/MAX_ROWS+1, no truncation')

    def test_financial_nine_mixed_positive_privacy_and_partial_scope(self):
        first = self.financial_base()
        self.assertEqual(first, self.financial_compare())
        self.assertEqual(len(first), 9)
        for i in range(9):
            financial = first[901+i]['financial']
            self.assertEqual(financial['path'], 'PERSONAL_LOT_V1' if i<6 else 'LEGACY_WALLET_ONLY')
            self.assertEqual(financial['counts']['reservations'], 1 if i<6 else 0)
            self.assertEqual(financial['counts']['ledger'], 1 if i<6 else 2)
            self.assertEqual(financial['counts']['lots'], 1)
            self.assertEqual(financial['counts']['billing_lifecycle_events'], 2 if i<6 else 3)
            self.assertEqual(financial['counts']['billing_history'], 0)
            self.assertEqual(financial['counts']['account'], 1)
            self.assertEqual(financial['counts']['wallet'], 1)
        encoded = core.canonical(first)
        lot_fields = {row[0]: row[2] for row in first[901]['financial']['families']['lots'][0]}
        self.assertIsNone(lot_fields['expires_at'])
        self.assertIsNone(lot_fields['policy_version_id'])
        for private in (b'SENSITIVE_SYNTHETIC', b'https://', b'synthetic-user-0', b'synthetic-account-0'):
            self.assertNotIn(private, encoded)
        self.assertEqual(first, core.sample_core_financial_read_only(self.db, list(reversed(self.entries))))
        with self.assertRaisesRegex(core.SnapshotError, 'CORE_SCOPE_INVALID'):
            core.core_sha256(first[901])
        self.final_reject(self.signed(), 'SNAPSHOT_SHA256_MISMATCH')
        self.assertEqual(core.MISSING_FAMILIES, ())

    def test_financial_unknown_key_status_owner_blocks_before_incomplete(self):
        self.financial_base()
        cases = [
            ("UPDATE public.xz_wallet_ledger SET idempotency_key='unknown' WHERE id='synthetic-ledger-6'", 'FINANCIAL_LEDGER_KEY_INVALID'),
            ("UPDATE public.xz_wallet_ledger SET user_id='other' WHERE id='synthetic-ledger-0'", 'FINANCIAL_LEDGER_LINKAGE_INVALID'),
            ("UPDATE public.xz_wallet_ledger SET task_id='other' WHERE id='synthetic-ledger-0'", 'FINANCIAL_LEDGER_LINKAGE_INVALID'),
            ("UPDATE public.xz_wallet_ledger SET reference_id='other' WHERE id='synthetic-ledger-0'", 'FINANCIAL_LEDGER_LINKAGE_INVALID'),
            ("UPDATE public.xz_generation_tasks SET billing_account_id='other' WHERE id='synthetic-task-0'", 'FINANCIAL_ACCOUNT_LINKAGE_INVALID'),
            ("UPDATE public.xz_generation_tasks SET raw=jsonb_set(raw,'{billingEngine}','\"unknown\"') WHERE id='synthetic-task-0'", 'FINANCIAL_MARKER_INVALID'),
            ("UPDATE public.xz_generation_tasks SET raw=jsonb_set(raw,'{personalPointReservationId}','null') WHERE id='synthetic-task-0'", 'FINANCIAL_TASK_LINKAGE_INVALID'),
            ("UPDATE public.xz_personal_point_reservations SET business_id='other' WHERE id='synthetic-reservation-0'", 'FINANCIAL_RESERVATION_LINKAGE_INVALID'),
            ("UPDATE public.xz_personal_point_reservations SET idempotency_key='unknown' WHERE id='synthetic-reservation-0'", 'FINANCIAL_RESERVATION_LINKAGE_INVALID'),
            ("INSERT INTO public.xz_point_accounts(id,user_id) VALUES('second','synthetic-user-0')", 'FINANCIAL_ACCOUNT_COUNT'),
            ("DELETE FROM public.xz_user_wallets WHERE user_id='synthetic-user-0'", 'FINANCIAL_WALLET_COUNT'),
            ("DELETE FROM public.xz_wallet_ledger WHERE id='synthetic-ledger-6'", 'FINANCIAL_RESERVE_COUNT'),
            ("UPDATE public.xz_wallet_ledger SET user_id=NULL WHERE id='synthetic-ledger-0'", 'FINANCIAL_LEDGER_LINKAGE_INVALID'),
            ("ALTER TABLE public.xz_personal_point_reservations DROP CONSTRAINT xz_personal_point_reservations_status_check; ALTER TABLE public.xz_personal_point_reservations DROP CONSTRAINT ck_xz_personal_point_reservations_status_balance; UPDATE public.xz_personal_point_reservations SET status='UNKNOWN' WHERE id='synthetic-reservation-0'", 'FINANCIAL_STATE_INVALID'),
            ("ALTER TABLE public.xz_wallet_ledger DROP CONSTRAINT xz_wallet_ledger_entry_type_check; ALTER TABLE public.xz_wallet_ledger DROP CONSTRAINT xz_wallet_ledger_transition_check; UPDATE public.xz_wallet_ledger SET entry_type='UNKNOWN' WHERE id='synthetic-ledger-0'", 'FINANCIAL_LEDGER_STATE_INVALID'),
        ]
        raw = self.signed()
        for statement,code in cases:
            with self.subTest(code=code,statement=statement.split(' ')[0]):
                with self.db.cursor() as cursor:
                    cursor.execute('BEGIN ISOLATION LEVEL REPEATABLE READ')
                    try:
                        cursor.execute(statement)
                        with self.assertRaisesRegex(core.SnapshotError, '^QUARANTINE_LIVE_'+code+'$'):
                            self.financial_compare(cursor)
                        with self.assertRaisesRegex(core.SnapshotError, '^QUARANTINE_LIVE_'+code+'$'):
                            core.validate_live_approval_in_transaction(cursor, raw, hashlib.sha256(raw).hexdigest(), 'b'*40, 'enroll')
                    finally:
                        cursor.execute('ROLLBACK')

    def test_financial_legacy_attributed_and_imported_runtime_keys(self):
        # Migration105 reserve attribution: no additional RESERVE movement.
        for imported in (False, True):
            with self.db.cursor() as cursor:
                cursor.execute('BEGIN ISOLATION LEVEL REPEATABLE READ')
                try:
                    cursor.execute('ALTER TABLE public.xz_personal_point_lot_movements DISABLE TRIGGER USER')
                    cursor.execute("DELETE FROM public.xz_personal_point_lot_movements WHERE id='synthetic-movement-0'")
                    key = 'synthetic-task-0:RESERVE'
                    metadata = {}
                    if imported:
                        key = 'personal-point:legacy-wallet:synthetic-account-0:synthetic-task-0:RESERVE:original-ledger'
                        metadata = {'legacy_ledger_id':'original-ledger'}
                    cursor.execute("UPDATE public.xz_wallet_ledger SET idempotency_key=%s,metadata=%s::jsonb WHERE id='synthetic-ledger-0'", (key,json.dumps(metadata)))
                    snapshots = core.project_core_financial_in_transaction(cursor, self.entries)
                    self.assertEqual(snapshots[901]['financial']['counts']['movements'], 0)
                    cursor.execute("UPDATE public.xz_wallet_ledger SET idempotency_key='unknown' WHERE id='synthetic-ledger-0'")
                    with self.assertRaisesRegex(core.SnapshotError, 'FINANCIAL_LEDGER_KEY_INVALID'):
                        core.project_core_financial_in_transaction(cursor, self.entries)
                finally:
                    cursor.execute('ROLLBACK')

    def test_financial_capture_release_and_all_history_not_latest(self):
        self.financial_base()
        for kind, command in [('CAPTURE','capture'),('RELEASE','release'),('RELEASE','durable-release')]:
            with self.subTest(kind=kind,command=command):
                with self.db.cursor() as cursor:
                    cursor.execute('BEGIN ISOLATION LEVEL REPEATABLE READ')
                    try:
                        capture = kind=='CAPTURE'
                        cursor.execute('ALTER TABLE public.xz_personal_point_lot_movements DISABLE TRIGGER USER')
                        cursor.execute("UPDATE public.xz_point_accounts SET available=%s,frozen=3 WHERE id='synthetic-account-0'", (100 if capture else 102,))
                        cursor.execute("UPDATE public.xz_user_wallets SET token_balance=%s,frozen_token=3,total_token_used=%s WHERE user_id='synthetic-user-0'", (100 if capture else 102,2 if capture else 0))
                        cursor.execute("UPDATE public.xz_personal_point_lots SET available_points=%s,reserved_points=3,consumed_points=%s WHERE id='synthetic-lot-0'", (100 if capture else 102,2 if capture else 0))
                        for table in ('xz_personal_point_reservations','xz_personal_point_reservation_allocations'):
                            cursor.execute('UPDATE public.'+table+" SET reserved_points=3,captured_points=%s,released_points=%s,status='PARTIAL' WHERE id=%s", (2 if capture else 0,0 if capture else 2,'synthetic-reservation-0' if table.endswith('reservations') else 'synthetic-allocation-0'))
                        cursor.execute("INSERT INTO public.xz_wallet_ledger(id,account_id,user_id,task_id,reference_type,reference_id,entry_type,points,available_before,available_after,frozen_before,frozen_after,idempotency_key) VALUES('terminal','synthetic-account-0','synthetic-user-0','synthetic-task-0','GENERATION_TASK','synthetic-task-0',%s,2,100,%s,5,3,%s)", (kind,100 if capture else 102,'personal-point:'+kind.lower()+':synthetic-account-0:generation:'+command+':synthetic-task-0'))
                        cursor.execute("INSERT INTO public.xz_personal_point_lot_movements(id,lot_id,account_id,user_id,movement_type,points,available_before,available_after,reserved_before,reserved_after,consumed_before,consumed_after,expired_before,expired_after,reversed_before,reversed_after,reservation_id,idempotency_key) VALUES('terminal','synthetic-lot-0','synthetic-account-0','synthetic-user-0',%s,2,100,%s,5,3,0,%s,0,0,0,0,'synthetic-reservation-0',%s)", (kind,100 if capture else 102,2 if capture else 0,kind.lower()+':generation:'+command+':synthetic-task-0:synthetic-lot-0'))
                        snapshots = core.project_core_financial_in_transaction(cursor, self.entries)
                        self.assertEqual(snapshots[901]['financial']['counts']['ledger'], 2)
                        self.assertEqual(snapshots[901]['financial']['counts']['movements'], 2)
                        with self.assertRaisesRegex(core.SnapshotError, 'PARTIAL_HASH_MISMATCH'):
                            self.financial_compare(cursor)
                        cursor.execute("DELETE FROM public.xz_wallet_ledger WHERE id='terminal'")
                        with self.assertRaisesRegex(core.SnapshotError, 'FINANCIAL_RESERVE_CONFLICT'):
                            core.project_core_financial_in_transaction(cursor, self.entries)
                    finally:
                        cursor.execute('ROLLBACK')

    def test_financial_every_family_field_drift_exact_predicates(self):
        self.insert_billing_history()
        self.financial_base()
        selectors = {'xz_point_accounts':('id','synthetic-account-0'), 'xz_user_wallets':('user_id','synthetic-user-0'),
                     'xz_personal_point_lots':('id','synthetic-lot-0'), 'xz_personal_point_reservations':('id','synthetic-reservation-0'),
                     'xz_personal_point_reservation_allocations':('id','synthetic-allocation-0'),
                     'xz_personal_point_lot_movements':('id','synthetic-movement-0'), 'xz_wallet_ledger':('id','synthetic-ledger-0'),
                     'xz_billing_lifecycle_events':('id','synthetic-event-0QUOTE'),
                     'xz_billing_events':('id','synthetic-history')}
        special = {
            ('xz_point_accounts','id'):'FINANCIAL_OWNER_MISMATCH', ('xz_point_accounts','user_id'):'FINANCIAL_OWNER_MISMATCH',
            ('xz_point_accounts','available'):'FINANCIAL_BALANCE_CONFLICT', ('xz_point_accounts','frozen'):'FINANCIAL_BALANCE_CONFLICT',
            ('xz_user_wallets','user_id'):'FINANCIAL_WALLET_COUNT', ('xz_user_wallets','token_balance'):'FINANCIAL_BALANCE_CONFLICT', ('xz_user_wallets','frozen_token'):'FINANCIAL_BALANCE_CONFLICT',
        }
        tested=0
        for table,columns in sorted(core.FINANCIAL_SCHEMA.items()):
            for column,typ,nullable,_,__,___ in columns:
                code = special.get((table,column),'PARTIAL_HASH_MISMATCH')
                if table=='xz_personal_point_reservations':
                    if column in ('id','account_id','user_id','business_type','business_id','idempotency_key'): code='FINANCIAL_RESERVATION_LINKAGE_INVALID'
                    elif column in ('requested_points','captured_points','released_points'): code='FINANCIAL_RESERVE_CONFLICT'
                    elif column in ('reserved_points','expired_points'): code='FINANCIAL_STATE_INVALID'
                    elif column=='status': code='FINANCIAL_STATE_INVALID'
                elif table=='xz_personal_point_reservation_allocations':
                    if column in ('account_id','user_id'): code='FINANCIAL_OWNER_MISMATCH'
                    elif column in ('allocated_points','reserved_points','captured_points','released_points','expired_points','status'): code='FINANCIAL_STATE_INVALID'
                    elif column in ('reservation_id','lot_id'): code='FINANCIAL_ALLOCATION_LINKAGE_INVALID'
                elif table=='xz_personal_point_lots':
                    if column in ('account_id','user_id'): code='FINANCIAL_OWNER_MISMATCH'
                    elif column in ('original_points','available_points','reserved_points','consumed_points','expired_points','reversed_points','status','source_type'): code='FINANCIAL_STATE_INVALID'
                    elif column=='id': code='FINANCIAL_ALLOCATION_LINKAGE_INVALID'
                    elif column=='expires_at': code='FINANCIAL_STATE_INVALID'
                elif table=='xz_personal_point_lot_movements':
                    if column in ('account_id','user_id'): code='FINANCIAL_OWNER_MISMATCH'
                    elif column in ('movement_type',): code='FINANCIAL_STATE_INVALID'
                    elif column in ('lot_id','reservation_id'): code='FINANCIAL_MOVEMENT_LINKAGE_INVALID'
                    elif column=='idempotency_key': code='FINANCIAL_MOVEMENT_KEY_INVALID'
                    elif column in ('points','available_before','available_after','reserved_before','reserved_after','consumed_before','consumed_after','expired_before','expired_after','reversed_before','reversed_after'): code='FINANCIAL_STATE_INVALID'
                elif table=='xz_wallet_ledger':
                    if column in ('account_id','user_id','task_id','reference_type','reference_id','tenant_id'): code='FINANCIAL_LEDGER_LINKAGE_INVALID'
                    elif column=='entry_type': code='FINANCIAL_LEDGER_STATE_INVALID'
                    elif column=='idempotency_key': code='FINANCIAL_LEDGER_KEY_INVALID'
                    elif column=='points': code='FINANCIAL_RESERVE_CONFLICT'
                    elif column in ('available_before','available_after','frozen_before','frozen_after'): code='FINANCIAL_STATE_INVALID'
                elif table=='xz_billing_events' and column in ('id','user_id','task_id','tenant_id'): code='FINANCIAL_HISTORY_LINKAGE_INVALID'
                elif table=='xz_billing_lifecycle_events' and column in ('user_id','task_id','tenant_id','event_type','billing_status','idempotency_key'): code='FINANCIAL_EVENT_LINKAGE_INVALID'
                with self.subTest(table=table,column=column,code=code):
                    with self.db.cursor() as cursor:
                        cursor.execute('BEGIN ISOLATION LEVEL REPEATABLE READ')
                        try:
                            # Drop constraints ONLY in our verified owned synthetic
                            # fixture and rollback each case, to exercise predicates
                            # even if a damaged DB has lost relational constraints.
                            for fixture_table in core.FINANCIAL_SCHEMA:
                                cursor.execute("SELECT conname FROM pg_constraint WHERE conrelid=%s::regclass", ('public.'+fixture_table,))
                                from psycopg2 import sql
                                for (constraint,) in cursor.fetchall():
                                    cursor.execute(sql.SQL('ALTER TABLE public.{} DROP CONSTRAINT IF EXISTS {} CASCADE').format(sql.Identifier(fixture_table),sql.Identifier(constraint)))
                            cursor.execute('ALTER TABLE public.xz_personal_point_lot_movements DISABLE TRIGGER USER')
                            cursor.execute('ALTER TABLE public.xz_personal_point_lots DISABLE TRIGGER USER')
                            key,value=selectors[table]
                            if typ=='text': expression="coalesce("+column+",'') || '-drift'"
                            elif typ in ('int8','numeric'): expression=column+'+1'
                            elif typ=='jsonb': expression="jsonb_build_object('syntheticDrift',true)"
                            elif typ=='timestamptz': expression="coalesce("+column+",'2000-01-01'::timestamptz)+interval '1 second'"
                            else: raise AssertionError('uncovered financial type')
                            cursor.execute('UPDATE public.'+table+' SET '+column+'='+expression+' WHERE '+key+'=%s', (value,))
                            with self.assertRaisesRegex(core.SnapshotError, '^QUARANTINE_LIVE_'+code+'$'):
                                self.financial_compare(cursor)
                            tested+=1
                        finally:
                            cursor.execute('ROLLBACK')
        self.assertEqual(tested,sum(len(value) for value in core.FINANCIAL_SCHEMA.values()))
        print('FINANCIAL_FIELD_DRIFT_CASES=%d exact predicates; no skips' % tested)

    def test_financial_schema_duplicates_and_insertion_deletion(self):
        self.financial_base()
        cases=[
            ('ALTER TABLE public.xz_user_wallets ADD COLUMN unreviewed text','SCHEMA_MISMATCH'),
            ('ALTER TABLE public.xz_wallet_ledger RENAME TO missing_ledger','SCHEMA_MISMATCH'),
            # Change a type without breaking expires_at>granted_at CHECK during
            # fixture setup; the real projection must reach SCHEMA_MISMATCH.
            ('ALTER TABLE public.xz_user_wallets ALTER COLUMN cash_balance_cents TYPE numeric','SCHEMA_MISMATCH'),
            ('ALTER TABLE public.xz_point_accounts ALTER COLUMN available DROP NOT NULL; UPDATE public.xz_point_accounts SET available=NULL','SCHEMA_MISMATCH'),
            ("ALTER TABLE public.xz_user_wallets DROP CONSTRAINT xz_user_wallets_pkey; INSERT INTO public.xz_user_wallets SELECT * FROM public.xz_user_wallets WHERE user_id='synthetic-user-0'",'FINANCIAL_WALLET_COUNT'),
            ("ALTER TABLE public.xz_wallet_ledger DROP CONSTRAINT xz_wallet_ledger_pkey; INSERT INTO public.xz_wallet_ledger SELECT (jsonb_populate_record(NULL::public.xz_wallet_ledger,to_jsonb(l)||'{\"idempotency_key\":\"synthetic-task-0:RESERVE\"}')).* FROM public.xz_wallet_ledger l WHERE id='synthetic-ledger-0'",'FINANCIAL_RESERVE_COUNT'),
            ("DELETE FROM public.xz_personal_point_reservation_allocations WHERE id='synthetic-allocation-0'",'FINANCIAL_ALLOCATION_COUNT_OR_TOTAL'),
            ("ALTER TABLE public.xz_personal_point_reservation_allocations DROP CONSTRAINT xz_personal_point_reservation_allocations_pkey; DO $$ DECLARE c record; BEGIN FOR c IN SELECT conname FROM pg_constraint WHERE conrelid='public.xz_personal_point_reservation_allocations'::regclass AND contype='u' LOOP EXECUTE format('ALTER TABLE public.xz_personal_point_reservation_allocations DROP CONSTRAINT %I',c.conname); END LOOP; END $$; INSERT INTO public.xz_personal_point_reservation_allocations SELECT * FROM public.xz_personal_point_reservation_allocations WHERE id='synthetic-allocation-0'",'FINANCIAL_DUPLICATE_IDENTITY'),
            ("INSERT INTO public.xz_point_accounts(id,user_id) VALUES('other-account','other-user'); INSERT INTO public.xz_personal_point_reservations(id,account_id,user_id,business_type,business_id,requested_points,reserved_points,idempotency_key) VALUES('conflict','other-account','other-user','GENERATION_TASK','synthetic-task-0',5,5,'other-key')",'FINANCIAL_RESERVATION_COUNT'),
            ("ALTER TABLE public.xz_personal_point_lot_movements DISABLE TRIGGER USER; DELETE FROM public.xz_personal_point_lot_movements WHERE id='synthetic-movement-0'",'FINANCIAL_MOVEMENT_COUNT_OR_TOTAL'),
            ("DELETE FROM public.xz_billing_lifecycle_events WHERE id='synthetic-event-0QUOTE'",'PARTIAL_HASH_MISMATCH'),
            ("INSERT INTO public.xz_wallet_ledger(id,account_id,user_id,entry_type,points,available_before,available_after,frozen_before,frozen_after,idempotency_key) VALUES('historical','synthetic-account-0','synthetic-user-0','GRANT',1,0,1,0,0,'historical-grant')",'PARTIAL_HASH_MISMATCH'),
        ]
        cases.extend(('ALTER TABLE public.'+table+' RENAME TO missing_financial_table','SCHEMA_MISMATCH')
                     for table in core.FINANCIAL_SCHEMA)
        for statement,code in cases:
            with self.subTest(code=code,statement=statement.split(';')[0]):
                with self.db.cursor() as cursor:
                    cursor.execute('BEGIN ISOLATION LEVEL REPEATABLE READ')
                    try:
                        cursor.execute(statement)
                        with self.assertRaisesRegex(core.SnapshotError, '^QUARANTINE_LIVE_'+code+'$'):
                            self.financial_compare(cursor)
                    finally:
                        cursor.execute('ROLLBACK')

    def test_financial_same_cursor_read_only_concurrency(self):
        self.financial_base()
        import psycopg2
        writer=psycopg2.connect(host='127.0.0.1',dbname='xianzhi_test',user='postgres',password='test_secret_pass')
        writer.autocommit=True
        try:
            with self.db.cursor() as cursor:
                cursor.execute('BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY')
                try:
                    first=self.financial_compare(cursor)
                    with writer.cursor() as other:
                        other.execute("UPDATE public.xz_user_wallets SET cash_balance_cents=99 WHERE user_id='synthetic-user-0'")
                    self.assertEqual(first,self.financial_compare(cursor))
                    self.assertEqual(first[901]['core'],core.project_core_in_transaction(cursor,self.entries)[901])
                    cursor.execute('SHOW transaction_read_only')
                    self.assertEqual(cursor.fetchone(),('on',))
                finally:
                    cursor.execute('ROLLBACK')
            self.financial_reject('PARTIAL_HASH_MISMATCH')
            with self.db.cursor() as cursor:
                with self.assertRaisesRegex(core.SnapshotError,'TRANSACTION_REQUIRED'):
                    core.project_core_financial_in_transaction(cursor,self.entries)
        finally:
            writer.close()

    def test_core_source_loader_ignores_timestamp_valid_pyc(self):
        with tempfile.TemporaryDirectory(prefix='synthetic-core-cache-') as directory:
            directory = Path(directory)
            for name in ('enroll-quarantine.py', 'quarantine-approval.py', 'quarantine-live-snapshot.py', 'quarantine-psql-transport.py'):
                shutil.copy2(str(ROOT / 'ops' / name), str(directory / name))
            source = directory / 'quarantine-live-snapshot.py'
            original, info = source.read_bytes(), source.stat()
            marker = directory / 'POISONED_CORE_EXECUTED'
            payload = ('open(' + repr(str(marker)) + ',"w").write("poison")\nraise SystemExit(0)\n').encode()
            source.write_bytes(payload + b'#'*(len(original)-len(payload)))
            os.utime(str(source), (info.st_atime, info.st_mtime))
            py_compile.compile(str(source), doraise=True)
            source.write_bytes(original)
            os.utime(str(source), (info.st_atime, info.st_mtime))
            control = 'import importlib.util,sys;s=importlib.util.spec_from_file_location("probe",sys.argv[1]);m=importlib.util.module_from_spec(s);s.loader.exec_module(m)'
            subprocess.run([sys.executable, '-B', '-c', control, str(source)], check=True)
            self.assertTrue(marker.exists())
            marker.unlink()
            proc = subprocess.run([sys.executable, str(directory / 'enroll-quarantine.py')], stdout=subprocess.PIPE, stderr=subprocess.PIPE)
            self.assertNotEqual(proc.returncode, 0)
            self.assertIn(b'Usage:', proc.stderr)
            self.assertFalse(marker.exists())

    def test_unsigned_candidate_is_fresh_canonical_and_never_signed_by_agent(self):
        now = approval_mod.datetime.datetime.now(approval_mod.datetime.timezone.utc)
        not_before = (now - approval_mod.datetime.timedelta(minutes=5)).strftime('%Y-%m-%dT%H:%M:%S.%fZ')
        expires_at = (now + approval_mod.datetime.timedelta(minutes=30)).strftime('%Y-%m-%dT%H:%M:%S.%fZ')
        candidate = core.sample_unsigned_candidate_read_only(
            self.db, self.entries, 'b'*40, 'UNPROVISIONED', not_before, expires_at)
        self.assertEqual(candidate['status'], 'UNSIGNED_REQUIRES_HUMAN_REVIEW')
        self.assertEqual(candidate['authority_id'], approval_mod.AUTHORITY_ID)
        self.assertEqual(candidate['record_count'], len(self.entries))
        self.assertEqual(len(candidate['records']), len(self.entries))
        self.assertNotIn('signature', candidate)
        self.assertNotIn('approval_id', candidate['records'][0])
        encoded = approval_mod.canonical(candidate)
        for private in (b'SENSITIVE_SYNTHETIC', b'https://', b'synthetic-user-0'):
            self.assertNotIn(private, encoded)
        for record in candidate['records']:
            self.assertEqual(record['snapshot_sha256'],
                             core.canonical_live_snapshot_sha256(record['snapshot']))

    def test_unsigned_candidate_includes_artwork_results_and_storage_families(self):
        self.stored_asset_fixture()
        now = approval_mod.datetime.datetime.now(approval_mod.datetime.timezone.utc)
        not_before = (now - approval_mod.datetime.timedelta(minutes=5)).strftime('%Y-%m-%dT%H:%M:%S.%fZ')
        expires_at = (now + approval_mod.datetime.timedelta(minutes=30)).strftime('%Y-%m-%dT%H:%M:%S.%fZ')
        candidate = core.sample_unsigned_candidate_read_only(
            self.db, self.entries, 'b'*40, 'UNPROVISIONED', not_before, expires_at)
        record = next(item for item in candidate['records'] if item['identity']['execution_id'] == 901)
        snapshot = record['snapshot']
        self.assertEqual(snapshot['asset_storage']['counts']['assets'], 1)
        self.assertEqual(snapshot['asset_storage']['counts']['files'], 1)
        self.assertEqual(snapshot['core']['counts']['attempts'], 1)
        self.assertEqual(snapshot['financial']['path'], 'PERSONAL_LOT_V1')
        self.assertEqual(record['snapshot_sha256'], core.canonical_live_snapshot_sha256(snapshot))

    def test_canonical_live_snapshot_positive_and_drift_negatives(self):
        raw = self.signed_canonical()
        with self.db.cursor() as cursor:
            cursor.execute('BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY')
            try:
                manifest = core.validate_live_snapshot_in_transaction(cursor, raw, 'b'*40, 'enroll')
                self.assertEqual(len(manifest['executions']), 9)
                approval = core.validate_live_approval_in_transaction(
                    cursor, raw, hashlib.sha256(raw).hexdigest(), 'b'*40, 'enroll')
                self.assertEqual(len(approval['executions']), 9)
            finally:
                cursor.execute('ROLLBACK')

        # Negative: tampered snapshot sha fails closed
        tampered_manifest = approval_mod.decode(raw)
        tampered_manifest['executions'][0]['snapshot_sha256'] = '0' * 64
        tampered_manifest['executions'][0]['evidence']['snapshot_sha256'] = '0' * 64
        tampered_manifest['executions'][0]['evidence_sha256'] = hashlib.sha256(core.canonical(tampered_manifest['executions'][0]['evidence'])).hexdigest()
        fixture = self.approval_class('test_valid_pinned_3072_signature_and_both_operations')
        fixture.setUp()
        signed_tampered = fixture.signed(tampered_manifest)
        with self.db.cursor() as cursor:
            cursor.execute('BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY')
            try:
                with self.assertRaisesRegex(core.SnapshotError, 'SNAPSHOT_SHA256_MISMATCH'):
                    core.validate_live_snapshot_in_transaction(cursor, signed_tampered, 'b'*40, 'enroll')
            finally:
                cursor.execute('ROLLBACK')

    def test_atomic_enrollment_full_positive_with_pre_commit_readback(self):
        self.sql('TRUNCATE public.provider_execution_quarantine CASCADE')
        raw = self.signed_canonical()
        count = enroll.enroll_quarantine(self.db, raw, 'b'*40)
        self.assertEqual(count, 9)

        with self.db.cursor() as cursor:
            cursor.execute('SELECT count(*) FROM public.provider_execution_quarantine')
            self.assertEqual(cursor.fetchone()[0], 9)
            cursor.execute("""
            SELECT execution_id, task_id, attempt, generation,
                   snapshot_sha256, evidence_sha256, approval_id, release_sha,
                   to_char(not_before AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),
                   to_char(expires_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')
            FROM public.provider_execution_quarantine
            ORDER BY execution_id;
            """)
            rows = cursor.fetchall()
            self.assertEqual(len(rows), 9)
            manifest = approval_mod.decode(raw)
            for row, exp in zip(rows, manifest['executions']):
                self.assertEqual(row[0], exp['execution_id'])
                self.assertEqual(row[1], exp['task_id'])
                self.assertEqual(row[2], exp['attempt'])
                self.assertEqual(row[3], exp['generation'])
                self.assertEqual(row[4], exp['snapshot_sha256'])
                self.assertEqual(row[5], exp['evidence_sha256'])
                self.assertEqual(row[6], exp['approval_id'])
                self.assertEqual(row[7], exp['release_sha'])
                self.assertEqual(row[8], exp['not_before'])
                self.assertEqual(row[9], exp['expires_at'])

    def test_atomic_enrollment_core_drift_negatives_rollback_zero_rows(self):
        raw = self.signed_canonical()
        cases = [
            ("UPDATE public.provider_executions SET task_execution_generation=999 WHERE id=901",
             "UPDATE public.provider_executions SET task_execution_generation=1 WHERE id=901"),
            ("UPDATE public.provider_executions SET attempt=5 WHERE id=901",
             "UPDATE public.provider_executions SET attempt=1 WHERE id=901"),
            ("UPDATE public.provider_executions SET status='processing' WHERE id=901",
             "UPDATE public.provider_executions SET status='unknown' WHERE id=901"),
            ("UPDATE public.xz_generation_tasks SET execution_generation=999 WHERE id='synthetic-task-0'",
             "UPDATE public.xz_generation_tasks SET execution_generation=2 WHERE id='synthetic-task-0'"),
            ("UPDATE public.xz_generation_tasks SET status='RUNNING', task_status='RUNNING' WHERE id='synthetic-task-0'",
             "UPDATE public.xz_generation_tasks SET status='FAILED', task_status='FAILED' WHERE id='synthetic-task-0'"),
        ]
        for drift_sql, restore_sql in cases:
            with self.subTest(drift=drift_sql):
                self.sql('TRUNCATE public.provider_execution_quarantine CASCADE')
                self.sql(drift_sql)
                try:
                    with self.assertRaises(enroll.EnrollmentError):
                        enroll.enroll_quarantine(self.db, raw, 'b'*40)
                    with self.db.cursor() as cursor:
                        cursor.execute('SELECT count(*) FROM public.provider_execution_quarantine')
                        self.assertEqual(cursor.fetchone()[0], 0, "Rollback must leave 0 rows on core drift")
                finally:
                    self.sql(restore_sql)

    def test_atomic_enrollment_financial_drift_negatives_rollback_zero_rows(self):
        raw = self.signed_canonical()
        cases = [
            ("UPDATE public.xz_point_accounts SET available=999 WHERE id='synthetic-account-0'",
             "UPDATE public.xz_point_accounts SET available=100 WHERE id='synthetic-account-0'"),
            ("UPDATE public.xz_personal_point_lots SET available_points=99,reserved_points=6 WHERE id='synthetic-lot-0'",
             "UPDATE public.xz_personal_point_lots SET available_points=100,reserved_points=5 WHERE id='synthetic-lot-0'"),
            ("UPDATE public.xz_wallet_ledger SET idempotency_key='unknown' WHERE id='synthetic-ledger-0'",
             "UPDATE public.xz_wallet_ledger SET idempotency_key='personal-point:reserve:synthetic-account-0:generation:reserve:synthetic-task-0' WHERE id='synthetic-ledger-0'"),
        ]
        for drift_sql, restore_sql in cases:
            with self.subTest(drift=drift_sql):
                self.sql('TRUNCATE public.provider_execution_quarantine CASCADE')
                self.sql(drift_sql)
                try:
                    with self.assertRaises(enroll.EnrollmentError):
                        enroll.enroll_quarantine(self.db, raw, 'b'*40)
                    with self.db.cursor() as cursor:
                        cursor.execute('SELECT count(*) FROM public.provider_execution_quarantine')
                        self.assertEqual(cursor.fetchone()[0], 0, "Rollback must leave 0 rows on financial drift")
                finally:
                    self.sql(restore_sql)

    def test_atomic_enrollment_asset_drift_negatives_rollback_zero_rows(self):
        self.stored_asset_fixture()
        raw = self.signed_canonical()
        cases = [
            ("UPDATE public.xz_file_objects SET file_size=999 WHERE file_id='file0'",
             "UPDATE public.xz_file_objects SET file_size=17 WHERE file_id='file0'"),
            ("UPDATE public.xz_assets SET favorite=true WHERE id='asset0'",
             "UPDATE public.xz_assets SET favorite=false WHERE id='asset0'"),
        ]
        for drift_sql, restore_sql in cases:
            with self.subTest(drift=drift_sql):
                self.sql('TRUNCATE public.provider_execution_quarantine CASCADE')
                self.sql(drift_sql)
                try:
                    with self.assertRaises(enroll.EnrollmentError):
                        enroll.enroll_quarantine(self.db, raw, 'b'*40)
                    with self.db.cursor() as cursor:
                        cursor.execute('SELECT count(*) FROM public.provider_execution_quarantine')
                        self.assertEqual(cursor.fetchone()[0], 0, "Rollback must leave 0 rows on asset drift")
                finally:
                    self.sql(restore_sql)

    def test_atomic_enrollment_readback_tamper_rollback_zero_rows(self):
        self.sql('TRUNCATE public.provider_execution_quarantine CASCADE')
        raw = self.signed_canonical()
        def tamper_hook(rows):
            tampered = list(rows)
            r0 = list(tampered[0])
            r0[4] = '0' * 64  # tampered snapshot_sha256
            tampered[0] = tuple(r0)
            return tampered
        with self.assertRaises(enroll.EnrollmentError):
            enroll.enroll_quarantine(self.db, raw, 'b'*40, readback_tamper_hook=tamper_hook)
        with self.db.cursor() as cursor:
            cursor.execute('SELECT count(*) FROM public.provider_execution_quarantine')
            self.assertEqual(cursor.fetchone()[0], 0, "Rollback must leave 0 rows on readback tamper")

    def test_atomic_enrollment_clock_window_negatives_rollback_zero_rows(self):
        self.sql('TRUNCATE public.provider_execution_quarantine CASCADE')
        expired_raw = self.signed_canonical(lambda m: m.update(expires_at='2000-01-01T00:00:00.000000Z'))
        with self.assertRaises(enroll.EnrollmentError):
            enroll.enroll_quarantine(self.db, expired_raw, 'b'*40)
        with self.db.cursor() as cursor:
            cursor.execute('SELECT count(*) FROM public.provider_execution_quarantine')
            self.assertEqual(cursor.fetchone()[0], 0, "Rollback must leave 0 rows on expired window")

        future_raw = self.signed_canonical(lambda m: m.update(not_before='2099-01-01T00:00:00.000000Z'))
        with self.assertRaises(enroll.EnrollmentError):
            enroll.enroll_quarantine(self.db, future_raw, 'b'*40)
        with self.db.cursor() as cursor:
            cursor.execute('SELECT count(*) FROM public.provider_execution_quarantine')
            self.assertEqual(cursor.fetchone()[0], 0, "Rollback must leave 0 rows on future window")

    def test_atomic_enrollment_hostile_task_id_injection_resistance(self):
        self.sql('TRUNCATE public.provider_execution_quarantine CASCADE')
        # Ensure parameterized statements safely handle hostile characters without SQL injection
        hostile_approval_id = "appr_$(whoami)`touch /tmp/pwned`'\"; DROP TABLE provider_execution_quarantine; --"
        raw = self.signed_canonical(lambda m: m['executions'][0].update(
            approval_id=hostile_approval_id,
            evidence=dict(m['executions'][0]['evidence'], approval_id=hostile_approval_id),
            evidence_sha256=hashlib.sha256(core.canonical(dict(m['executions'][0]['evidence'], approval_id=hostile_approval_id))).hexdigest(),
        ))
        count = enroll.enroll_quarantine(self.db, raw, 'b'*40)
        self.assertEqual(count, 9)
        with self.db.cursor() as cursor:
            cursor.execute('SELECT count(*) FROM public.provider_execution_quarantine')
            self.assertEqual(cursor.fetchone()[0], 9)
            cursor.execute('SELECT approval_id FROM public.provider_execution_quarantine WHERE execution_id=901')
            self.assertEqual(cursor.fetchone()[0], hostile_approval_id)



# Synthetic harness ownership only; no production image/approval selector.
IMAGE_OWNER_LABEL = 'issue203.synthetic-live-owner'


class OwnedSyntheticImage:
    def __init__(self):
        self.token = uuid.uuid4().hex
        self.tag = 'issue203-live-snapshot-synthetic-' + self.token + ':latest'
        self.image_id = None
        if self.inspect(self.tag) is not None:
            raise RuntimeError('refuse pre-existing synthetic image tag')

    @staticmethod
    def inspect(selector):
        proc = subprocess.run(['docker', 'image', 'inspect', selector],
                              stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        if proc.returncode:
            # Missing daemon/tool must fail, not masquerade as an absent tag.
            subprocess.run(['docker', 'info'], stdout=subprocess.DEVNULL,
                           stderr=subprocess.PIPE, check=True)
            return None
        rows = json.loads(proc.stdout.decode())
        if len(rows) != 1:
            raise RuntimeError('ambiguous synthetic image identity')
        return rows[0]

    def build(self):
        with tempfile.TemporaryDirectory(prefix='issue203-image-id-') as directory:
            iid = Path(directory) / 'image-id'
            args = ['docker', 'build', '--iidfile', str(iid), '--label',
                    IMAGE_OWNER_LABEL + '=' + self.token, '-t', self.tag]
            args += ['-f', str(ROOT / 'tests/issue203-live-snapshot.Dockerfile'), str(ROOT)]
            subprocess.run(args, check=True)
            self.image_id = iid.read_text().strip()
        self.validate()
        if self.inspect(self.tag)['Id'] != self.image_id:
            raise RuntimeError('synthetic build tag substituted')
        print('OWNED_IMAGE ' + json.dumps(dict(tag=self.tag, image_id=self.image_id,
                                               owner=self.token)), flush=True)

    def validate(self):
        if not self.image_id or not re.fullmatch(r'sha256:[0-9a-f]{64}', self.image_id):
            raise RuntimeError('invalid built synthetic image ID')
        info = self.inspect(self.image_id)
        if (info is None or info['Id'] != self.image_id or
                info['Config']['Labels'].get(IMAGE_OWNER_LABEL) != self.token):
            raise RuntimeError('synthetic image ownership mismatch')

    def run(self, options, command, **kwargs):
        self.validate()
        # Fixed built ID, NEVER the mutable tag. --rm owns only this container.
        args = ['docker', 'run', '--rm'] + options + [self.image_id] + command
        print('OWNED_CMD ' + json.dumps(args), flush=True)
        return subprocess.run(args, stdin=subprocess.DEVNULL, **kwargs)

    def cleanup(self):
        info = self.inspect(self.tag)
        if (info is not None and info['Id'] == self.image_id and
                info['Config']['Labels'].get(IMAGE_OWNER_LABEL) == self.token):
            # Untag only. Never rmi an ID (another run may reference it).
            subprocess.run(['docker', 'image', 'rm', self.tag], check=True,
                           stdout=subprocess.DEVNULL)


class SyntheticImageOwnershipTests(unittest.TestCase):
    def test_preexisting_tag_refused(self):
        from unittest import mock
        with mock.patch.object(uuid, 'uuid4') as identifier:
            identifier.return_value.hex = self.image.token
            with self.assertRaisesRegex(RuntimeError, 'pre-existing'):
                OwnedSyntheticImage()
        self.assertEqual(OwnedSyntheticImage.inspect(self.image.tag)['Id'], self.image.image_id)

    def test_concurrent_tag_and_retag_do_not_substitute_runtime_or_cleanup(self):
        other = OwnedSyntheticImage()
        try:
            other.build()  # Same cached test layers, distinct run ownership label.
            # Concurrent run can reference the same ID under its own tag.
            alias = 'issue203-live-snapshot-synthetic-alias-' + other.token + ':latest'
            subprocess.run(['docker', 'tag', self.image.image_id, alias], check=True)
            try:
                self.image.cleanup()
                self.assertEqual(OwnedSyntheticImage.inspect(alias)['Id'], self.image.image_id)
                self.assertEqual(OwnedSyntheticImage.inspect(other.tag)['Id'], other.image_id)
                # Substitute run A's tag with B's different ID; A must still run A.
                subprocess.run(['docker', 'tag', other.image_id, self.image.tag], check=True)
                from unittest import mock
                with mock.patch.object(subprocess, 'run', wraps=subprocess.run) as calls:
                    proc = self.image.run(['--network', 'none'], ['python', '-c',
                        'import sys,ssl,psycopg2;print(sys.version);print(ssl.OPENSSL_VERSION);print(psycopg2.__version__)'],
                        stdout=subprocess.PIPE, stderr=subprocess.PIPE)
                    actual_args = calls.call_args[0][0]
                    self.assertIn(self.image.image_id, actual_args)
                    self.assertNotIn(self.image.tag, actual_args)
                    self.assertNotIn(other.image_id, actual_args)
                self.assertEqual(proc.returncode, 0, proc.stderr)
                built_id = self.image.image_id
                try:
                    self.image.image_id = other.image_id
                    with self.assertRaisesRegex(RuntimeError, 'ownership mismatch'):
                        self.image.validate()
                finally:
                    self.image.image_id = built_id
                print('ACTUAL_SYNTHETIC_RUNTIME ' + proc.stdout.decode(), flush=True)
                self.image.cleanup()
                self.assertEqual(OwnedSyntheticImage.inspect(self.image.tag)['Id'], other.image_id)
                other.run(['--network', 'none'], ['python', '-c', 'print("other-run-alive")'], check=True)
                # Restore our tag for final owned cleanup; do not remove B's tag/ID.
                subprocess.run(['docker', 'tag', self.image.image_id, self.image.tag], check=True)
            finally:
                subprocess.run(['docker', 'image', 'rm', alias], check=True, stdout=subprocess.DEVNULL)
        finally:
            other.cleanup()


def host_harness():
    image = OwnedSyntheticImage()
    try:
        image.build()
        SyntheticImageOwnershipTests.image = image
        ownership = unittest.TextTestRunner(verbosity=2).run(
            unittest.defaultTestLoader.loadTestsFromTestCase(SyntheticImageOwnershipTests))
        print('IMAGE_OWNERSHIP_TEST_EXIT skipped=%d tests=%d' % (len(ownership.skipped), ownership.testsRun), flush=True)
        if not ownership.wasSuccessful() or ownership.skipped or ownership.testsRun != 2:
            return 1
        fixture = source_module('owned_fixture', ROOT / 'tests/issue203-control-plane-test.py')
        owned = fixture.IsolatedPostgresControlPlaneTests
        owned.setUpClass()
        try:
            token = uuid.uuid4().hex
            subprocess.run(['docker', 'exec', '-i', owned.container, 'psql', '-X', '-U', 'postgres', '-d', 'xianzhi_test', '-v', 'ON_ERROR_STOP=1'], input=("CREATE TABLE public.issue203_owned_live_test(token text NOT NULL); INSERT INTO public.issue203_owned_live_test VALUES('%s');" % token).encode(), stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=True)
            info = subprocess.check_output(['docker', 'inspect', owned.container])
            print('ACTUAL_PG_IMAGE ' + json.loads(info.decode())[0]['Image'], flush=True)
            # PG --network none: only this owned loopback namespace is shared.
            return image.run(['--network', 'container:' + owned.container,
                              '-v', str(ROOT) + ':/source:ro',
                              '-e', 'ISSUE203_OWNED_LIVE_DB=' + token],
                             ['python', 'tests/issue203-live-snapshot-test.py', '--inside']).returncode
        finally:
            owned.tearDownClass()
    finally:
        image.cleanup()


if __name__ == '__main__':
    if sys.argv[1:] == ['--inside']:
        result = unittest.TextTestRunner(verbosity=2).run(unittest.defaultTestLoader.loadTestsFromTestCase(CoreOnlyPostgresTests))
        print('CORE_ONLY_TEST_EXIT skipped=%d tests=%d; final-live-approval=BLOCKED' % (len(result.skipped), result.testsRun))
        sys.exit(0 if result.wasSuccessful() and not result.skipped and result.testsRun else 1)
    else:
        sys.exit(host_harness())
