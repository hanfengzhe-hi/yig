package storage

import (
	"net/url"
	"strings"
	"time"

	"github.com/journeymidnight/yig/api/datatype"
	"github.com/journeymidnight/yig/api/datatype/policy"
	. "github.com/journeymidnight/yig/context"
	. "github.com/journeymidnight/yig/error"
	"github.com/journeymidnight/yig/helper"
	"github.com/journeymidnight/yig/iam"
	"github.com/journeymidnight/yig/iam/common"
	meta "github.com/journeymidnight/yig/meta/types"
	"github.com/journeymidnight/yig/meta/util"
	"github.com/journeymidnight/yig/redis"
)

const (
	BUCKET_NUMBER_LIMIT = 100
)

func (yig *YigStorage) MakeBucket(reqCtx RequestContext, acl datatype.Acl,
	credential common.Credential) error {
	// Input validation.

	if reqCtx.BucketInfo != nil {
		helper.Logger.Info("Error get bucket:", reqCtx.BucketName, "with error:", ErrBucketAlreadyExists)
		return ErrBucketAlreadyExists
	}

	buckets, err := yig.MetaStorage.GetUserBuckets(credential.UserId, false)
	if err != nil {
		return err
	}
	if len(buckets)+1 > BUCKET_NUMBER_LIMIT {
		return ErrTooManyBuckets
	}

	now := time.Now().UTC()
	bucket := meta.Bucket{
		Name:       reqCtx.BucketName,
		CreateTime: now,
		OwnerId:    credential.UserId,
		ACL:        acl,
		Versioning: meta.VersionDisabled, // it's the default
	}
	err = yig.MetaStorage.Client.PutNewBucket(bucket)
	if err != nil {
		helper.Logger.Error("Error Put New Bucket:", err)
		return err
	}

	yig.MetaStorage.Cache.Remove(redis.UserTable, credential.UserId)
	return err
}

func (yig *YigStorage) SetBucketAcl(bucketName string, policy datatype.AccessControlPolicy, acl datatype.Acl,
	credential common.Credential) error {

	if acl.CannedAcl == "" {
		newCannedAcl, err := datatype.GetCannedAclFromPolicy(policy)
		if err != nil {
			return err
		}
		acl = newCannedAcl
	}

	bucket, err := yig.MetaStorage.GetBucket(bucketName, false)
	if err != nil {
		return err
	}
	if bucket.OwnerId != credential.UserId {
		return ErrBucketAccessForbidden
	}
	bucket.ACL = acl
	err = yig.MetaStorage.Client.PutBucket(*bucket)
	if err != nil {
		return err
	}
	if err == nil {
		yig.MetaStorage.Cache.Remove(redis.BucketTable, bucketName)
	}
	return nil
}

func (yig *YigStorage) SetBucketLifecycle(bucketName string, lc datatype.Lifecycle,
	credential common.Credential) error {
	helper.Logger.Info("enter SetBucketLifecycle")
	bucket, err := yig.MetaStorage.GetBucket(bucketName, true)
	if err != nil {
		return err
	}
	if bucket.OwnerId != credential.UserId {
		return ErrBucketAccessForbidden
	}
	bucket.Lifecycle = lc
	err = yig.MetaStorage.Client.PutBucket(*bucket)
	if err != nil {
		return err
	}
	if err == nil {
		yig.MetaStorage.Cache.Remove(redis.BucketTable, bucketName)
	}

	err = yig.MetaStorage.PutBucketToLifeCycle(*bucket)
	if err != nil {
		helper.Logger.Error("Error Put bucket to lifecycle table:", err)
		return err
	}
	return nil
}

func (yig *YigStorage) GetBucketLifecycle(bucketName string, credential common.Credential) (lc datatype.Lifecycle,
	err error) {
	bucket, err := yig.MetaStorage.GetBucket(bucketName, true)
	if err != nil {
		return lc, err
	}
	if bucket.OwnerId != credential.UserId {
		err = ErrBucketAccessForbidden
		return
	}
	if len(bucket.Lifecycle.Rule) == 0 {
		err = ErrNoSuchBucketLc
		return
	}
	return bucket.Lifecycle, nil
}

func (yig *YigStorage) DelBucketLifecycle(bucketName string, credential common.Credential) error {
	bucket, err := yig.MetaStorage.GetBucket(bucketName, true)
	if err != nil {
		return err
	}
	if bucket.OwnerId != credential.UserId {
		return ErrBucketAccessForbidden
	}
	bucket.Lifecycle = datatype.Lifecycle{}
	err = yig.MetaStorage.Client.PutBucket(*bucket)
	if err != nil {
		return err
	}
	if err == nil {
		yig.MetaStorage.Cache.Remove(redis.BucketTable, bucketName)
	}
	err = yig.MetaStorage.RemoveBucketFromLifeCycle(*bucket)
	if err != nil {
		helper.Logger.Error("Remove bucket From lifecycle table error:", err)
		return err
	}
	return nil
}

func (yig *YigStorage) SetBucketCors(bucketName string, cors datatype.Cors,
	credential common.Credential) error {

	bucket, err := yig.MetaStorage.GetBucket(bucketName, false)
	if err != nil {
		return err
	}
	if bucket.OwnerId != credential.UserId {
		return ErrBucketAccessForbidden
	}
	bucket.CORS = cors
	err = yig.MetaStorage.Client.PutBucket(*bucket)
	if err != nil {
		return err
	}
	if err == nil {
		yig.MetaStorage.Cache.Remove(redis.BucketTable, bucketName)
	}
	return nil
}

func (yig *YigStorage) DeleteBucketCors(bucketName string, credential common.Credential) error {
	bucket, err := yig.MetaStorage.GetBucket(bucketName, false)
	if err != nil {
		return err
	}
	if bucket.OwnerId != credential.UserId {
		return ErrBucketAccessForbidden
	}
	bucket.CORS = datatype.Cors{}
	err = yig.MetaStorage.Client.PutBucket(*bucket)
	if err != nil {
		return err
	}
	if err == nil {
		yig.MetaStorage.Cache.Remove(redis.BucketTable, bucketName)
	}
	return nil
}

func (yig *YigStorage) GetBucketCors(bucketName string,
	credential common.Credential) (cors datatype.Cors, err error) {

	bucket, err := yig.MetaStorage.GetBucket(bucketName, true)
	if err != nil {
		return cors, err
	}
	if bucket.OwnerId != credential.UserId {
		err = ErrBucketAccessForbidden
		return
	}
	if len(bucket.CORS.CorsRules) == 0 {
		err = ErrNoSuchBucketCors
		return
	}
	return bucket.CORS, nil
}

func (yig *YigStorage) SetBucketVersioning(bucketName string, versioning datatype.Versioning,
	credential common.Credential) error {

	bucket, err := yig.MetaStorage.GetBucket(bucketName, false)
	if err != nil {
		return err
	}
	if bucket.OwnerId != credential.UserId {
		return ErrBucketAccessForbidden
	}
	bucket.Versioning = versioning.Status
	err = yig.MetaStorage.Client.PutBucket(*bucket)
	if err != nil {
		return err
	}
	if err == nil {
		yig.MetaStorage.Cache.Remove(redis.BucketTable, bucketName)
	}
	return nil
}

func (yig *YigStorage) GetBucketVersioning(bucketName string, credential common.Credential) (
	versioning datatype.Versioning, err error) {

	bucket, err := yig.MetaStorage.GetBucket(bucketName, false)
	if err != nil {
		return versioning, err
	}
	versioning.Status = helper.Ternary(bucket.Versioning == meta.VersionDisabled,
		"", bucket.Versioning).(string)
	return
}

func (yig *YigStorage) GetBucketAcl(bucketName string, credential common.Credential) (
	policy datatype.AccessControlPolicyResponse, err error) {

	bucket, err := yig.MetaStorage.GetBucket(bucketName, false)
	if err != nil {
		return policy, err
	}
	if bucket.OwnerId != credential.UserId {
		err = ErrBucketAccessForbidden
		return
	}
	owner := datatype.Owner{ID: credential.UserId, DisplayName: credential.DisplayName}
	bucketOwner := datatype.Owner{}
	policy, err = datatype.CreatePolicyFromCanned(owner, bucketOwner, bucket.ACL)
	if err != nil {
		return policy, err
	}

	return
}

// For INTERNAL USE ONLY
func (yig *YigStorage) GetBucket(bucketName string) (*meta.Bucket, error) {
	return yig.MetaStorage.GetBucket(bucketName, true)
}

func (yig *YigStorage) GetBucketInfo(bucketName string,
	credential common.Credential) (bucket *meta.Bucket, err error) {

	bucket, err = yig.MetaStorage.GetBucket(bucketName, true)
	if err != nil {
		return
	}

	if !credential.AllowOtherUserAccess {
		if bucket.OwnerId != credential.UserId {
			switch bucket.ACL.CannedAcl {
			case "public-read", "public-read-write", "authenticated-read":
				break
			default:
				err = ErrBucketAccessForbidden
				return
			}
		}
	}

	return
}

func (yig *YigStorage) GetBucketInfoByCtx(ctx RequestContext,
	credential common.Credential) (bucket *meta.Bucket, err error) {

	bucket = ctx.BucketInfo
	if bucket == nil {
		return nil, ErrNoSuchBucket
	}
	if !credential.AllowOtherUserAccess {
		if bucket.OwnerId != credential.UserId {
			switch bucket.ACL.CannedAcl {
			case "public-read", "public-read-write", "authenticated-read":
				break
			default:
				err = ErrBucketAccessForbidden
				return
			}
		}
	}

	return
}

func (yig *YigStorage) SetBucketPolicy(credential common.Credential, bucketName string, bucketPolicy policy.Policy) (err error) {
	bucket, err := yig.MetaStorage.GetBucket(bucketName, false)
	if err != nil {
		return err
	}
	if bucket.OwnerId != credential.UserId {
		return ErrBucketAccessForbidden
	}
	data, err := bucketPolicy.MarshalJSON()
	if err != nil {
		return
	}
	p := string(data)
	// If policy is empty then delete the bucket policy.
	if p == "" {
		bucket.Policy = policy.Policy{}
	} else {
		bucket.Policy = bucketPolicy
	}

	err = yig.MetaStorage.Client.PutBucket(*bucket)
	if err != nil {
		return err
	}
	if err == nil {
		yig.MetaStorage.Cache.Remove(redis.BucketTable, bucketName)
	}
	return nil
}

func (yig *YigStorage) GetBucketPolicy(credential common.Credential, bucketName string) (bucketPolicy policy.Policy, err error) {
	bucket, err := yig.MetaStorage.GetBucket(bucketName, true)
	if err != nil {
		return
	}
	if bucket.OwnerId != credential.UserId {
		err = ErrBucketAccessForbidden
		return
	}

	policyBuf, err := bucket.Policy.MarshalJSON()
	if err != nil {
		return
	}
	p, err := policy.ParseConfig(strings.NewReader(string(policyBuf)), bucketName)
	if err != nil {
		return bucketPolicy, ErrMalformedPolicy
	}

	bucketPolicy = *p
	return
}

func (yig *YigStorage) DeleteBucketPolicy(credential common.Credential, bucketName string) error {
	bucket, err := yig.MetaStorage.GetBucket(bucketName, false)
	if err != nil {
		return err
	}
	if bucket.OwnerId != credential.UserId {
		return ErrBucketAccessForbidden
	}
	bucket.Policy = policy.Policy{}
	err = yig.MetaStorage.Client.PutBucket(*bucket)
	if err != nil {
		return err
	}
	if err == nil {
		yig.MetaStorage.Cache.Remove(redis.BucketTable, bucketName)
	}
	return nil
}

func (yig *YigStorage) SetBucketWebsite(bucket *meta.Bucket, config datatype.WebsiteConfiguration) (err error) {
	bucket.Website = config
	err = yig.MetaStorage.Client.PutBucket(*bucket)
	if err != nil {
		return err
	}
	yig.MetaStorage.Cache.Remove(redis.BucketTable, bucket.Name)
	return nil
}

func (yig *YigStorage) GetBucketWebsite(bucketName string) (config datatype.WebsiteConfiguration, err error) {
	bucket, err := yig.MetaStorage.GetBucket(bucketName, true)
	if err != nil {
		return
	}
	return bucket.Website, nil
}

func (yig *YigStorage) DeleteBucketWebsite(bucket *meta.Bucket) error {
	bucket.Website = datatype.WebsiteConfiguration{}
	err := yig.MetaStorage.Client.PutBucket(*bucket)
	if err != nil {
		return err
	}
	yig.MetaStorage.Cache.Remove(redis.BucketTable, bucket.Name)
	return nil
}

func (yig *YigStorage) ListBuckets(credential common.Credential) (buckets []meta.Bucket, err error) {
	bucketNames, err := yig.MetaStorage.GetUserBuckets(credential.UserId, true)
	if err != nil {
		return
	}
	for _, bucketName := range bucketNames {
		bucket, err := yig.MetaStorage.GetBucket(bucketName, true)
		if err != nil {
			return buckets, err
		}
		buckets = append(buckets, *bucket)
	}
	return
}

func (yig *YigStorage) DeleteBucket(reqCtx RequestContext, credential common.Credential) (err error) {
	bucket := reqCtx.BucketInfo
	if bucket == nil {
		return ErrNoSuchBucket
	}
	if bucket.OwnerId != credential.UserId {
		return ErrBucketAccessForbidden
		// TODO validate bucket policy
	}

	bucketName := reqCtx.BucketName

	isEmpty, err := yig.MetaStorage.Client.IsEmptyBucket(bucketName)
	if err != nil {
		return err
	}
	if !isEmpty {
		return ErrBucketNotEmpty
	}
	err = yig.MetaStorage.Client.DeleteBucket(*bucket)
	if err != nil {
		return err
	}

	if err == nil {
		yig.MetaStorage.Cache.Remove(redis.UserTable, credential.UserId)
		yig.MetaStorage.Cache.Remove(redis.BucketTable, bucketName)
	}

	if bucket.Lifecycle.Rule != nil {
		err = yig.MetaStorage.RemoveBucketFromLifeCycle(*bucket)
		if err != nil {
			helper.Logger.Warn("Remove bucket from lifeCycle error:", err)
		}
	}

	return nil
}

func (yig *YigStorage) ListObjectsInternal(bucketName string,
	request datatype.ListObjectsRequest) (info meta.ListObjectsInfo, err error) {

	var marker string
	if request.Versioned {
		marker = request.KeyMarker
	} else if request.Version == 2 {
		if request.ContinuationToken != "" {
			marker, err = util.DecryptToString(request.ContinuationToken)
			if err != nil {
				err = ErrInvalidContinuationToken
				return
			}
		} else {
			marker = request.StartAfter
		}
	} else { // version 1
		marker = request.Marker
	}
	helper.Logger.Info("Prefix:", request.Prefix, "Marker:", request.Marker, "MaxKeys:",
		request.MaxKeys, "Delimiter:", request.Delimiter, "Version:", request.Version,
		"keyMarker:", request.KeyMarker, "versionIdMarker:", request.VersionIdMarker)
	return yig.MetaStorage.Client.ListObjects(bucketName, marker, request.Prefix, request.Delimiter, request.MaxKeys)
}

func (yig *YigStorage) ListVersionedObjectsInternal(bucketName string,
	request datatype.ListObjectsRequest) (info meta.VersionedListObjectsInfo, err error) {

	var marker string
	var verIdMarker string
	if request.Versioned {
		marker = request.KeyMarker
		verIdMarker = request.VersionIdMarker
	} else if request.Version == 2 {
		if request.ContinuationToken != "" {
			marker, err = util.DecryptToString(request.ContinuationToken)
			if err != nil {
				err = ErrInvalidContinuationToken
				return
			}
		} else {
			marker = request.StartAfter
		}
	} else { // version 1
		marker = request.Marker
	}
	helper.Logger.Info("Prefix:", request.Prefix, "Marker:", request.Marker, "MaxKeys:",
		request.MaxKeys, "Delimiter:", request.Delimiter, "Version:", request.Version,
		"keyMarker:", request.KeyMarker, "versionIdMarker:", request.VersionIdMarker)
	return yig.MetaStorage.Client.ListVersionedObjects(bucketName, marker, verIdMarker, request.Prefix, request.Delimiter, request.MaxKeys)
}

func (yig *YigStorage) ListObjects(reqCtx RequestContext, credential common.Credential,
	request datatype.ListObjectsRequest) (result meta.ListObjectsInfo, err error) {

	bucket := reqCtx.BucketInfo
	if bucket == nil {
		return result, ErrNoSuchBucket
	}

	switch bucket.ACL.CannedAcl {
	case "public-read", "public-read-write":
		break
	case "authenticated-read":
		if credential.UserId == "" {
			err = ErrBucketAccessForbidden
			return
		}
	default:
		if bucket.OwnerId != credential.UserId {
			err = ErrBucketAccessForbidden
			return
		}
	}
	// TODO validate user policy and ACL

	info, err := yig.ListObjectsInternal(bucket.Name, request)
	if info.IsTruncated && len(info.NextMarker) != 0 {
		result.NextMarker = info.NextMarker
	}
	if request.Version == 2 {
		result.NextMarker = util.Encrypt([]byte(result.NextMarker))
	}
	objects := make([]datatype.Object, 0, len(info.Objects))
	for _, obj := range info.Objects {
		helper.Logger.Info("result:", obj)
		object := datatype.Object{
			LastModified: obj.LastModified,
			ETag:         "\"" + obj.ETag + "\"",
			Size:         obj.Size,
			StorageClass: "STANDARD",
		}
		if request.EncodingType != "" { // only support "url" encoding for now
			object.Key = url.QueryEscape(obj.Key)
		} else {
			object.Key = obj.Key
		}

		if request.FetchOwner {
			var owner common.Credential
			owner, err = iam.GetCredentialByUserId(obj.Owner.ID)
			if err != nil {
				return
			}
			object.Owner = datatype.Owner{
				ID:          owner.UserId,
				DisplayName: owner.DisplayName,
			}
		}
		objects = append(objects, object)
	}
	result.Objects = objects
	result.Prefixes = info.Prefixes
	result.IsTruncated = info.IsTruncated

	if request.EncodingType != "" { // only support "url" encoding for now
		result.Prefixes = helper.Map(result.Prefixes, func(s string) string {
			return url.QueryEscape(s)
		})
		result.NextMarker = url.QueryEscape(result.NextMarker)
	}
	return
}

// TODO: refactor, similar to ListObjects
// or not?
func (yig *YigStorage) ListVersionedObjects(reqCtx RequestContext, credential common.Credential,
	request datatype.ListObjectsRequest) (result meta.VersionedListObjectsInfo, err error) {

	bucket := reqCtx.BucketInfo
	if bucket == nil {
		return result, ErrNoSuchBucket
	}

	switch bucket.ACL.CannedAcl {
	case "public-read", "public-read-write":
		break
	case "authenticated-read":
		if credential.UserId == "" {
			err = ErrBucketAccessForbidden
			return
		}
	default:
		if bucket.OwnerId != credential.UserId {
			err = ErrBucketAccessForbidden
			return
		}
	}

	info, err := yig.ListVersionedObjectsInternal(bucket.Name, request)
	if info.IsTruncated && len(info.NextKeyMarker) != 0 {
		result.NextKeyMarker = info.NextKeyMarker
		result.NextVersionIdMarker = info.NextVersionIdMarker
	}

	objects := make([]datatype.VersionedObject, 0, len(info.Objects))
	for _, o := range info.Objects {
		// TODO: IsLatest
		object := datatype.VersionedObject{
			LastModified: o.LastModified,
			ETag:         "\"" + o.ETag + "\"",
			Size:         o.Size,
			StorageClass: "STANDARD",
			Key:          o.Key,
		}
		if request.EncodingType != "" { // only support "url" encoding for now
			object.Key = url.QueryEscape(object.Key)
		}
		object.VersionId = o.VersionId
		if o.DeleteMarker {
			object.XMLName.Local = "DeleteMarker"
		} else {
			object.XMLName.Local = "Version"
		}
		if request.FetchOwner {
			var owner common.Credential
			owner, err = iam.GetCredentialByUserId(o.Owner.ID)
			if err != nil {
				return
			}
			object.Owner = datatype.Owner{
				ID:          owner.UserId,
				DisplayName: owner.DisplayName,
			}
		}
		objects = append(objects, object)
	}
	result.Objects = objects
	result.Prefixes = info.Prefixes
	result.IsTruncated = info.IsTruncated

	if request.EncodingType != "" { // only support "url" encoding for now
		result.Prefixes = helper.Map(result.Prefixes, func(s string) string {
			return url.QueryEscape(s)
		})
		result.NextKeyMarker = url.QueryEscape(result.NextKeyMarker)
	}

	return
}
