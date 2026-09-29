// Module seaweedfs holds the S3 helpers object-store lessons repeat. It lives
// beside lab.ts rather than in it so a lesson that never talks to SeaweedFS
// does not load the AWS SDK.
import {
  CreateBucketCommand,
  DeleteObjectsCommand,
  HeadObjectCommand,
  ListObjectsV2Command,
  S3Client,
} from "@aws-sdk/client-s3";
import { env } from "lab/lab.ts";

/**
 * DEFAULT_URL is the local dev S3 gateway from software/software.md. The
 * credentials are not secret; every lesson uses them as-is.
 */
export const DEFAULT_URL = "http://localhost:8333";
export const ACCESS_KEY_ID = "trinkets";
export const SECRET_ACCESS_KEY = "trinkets-secret";

/** url is OBJECT_STORE_URL, or DEFAULT_URL. */
export function url(): string {
  return env("OBJECT_STORE_URL", DEFAULT_URL);
}

/**
 * connect builds a client for url(). SeaweedFS accepts any region and needs
 * path-style addressing (bucket in the path, not a subdomain). The SDK
 * contacts the server only on the first command.
 */
export function connect(): S3Client {
  return new S3Client({
    endpoint: url(),
    region: "us-east-1",
    forcePathStyle: true,
    credentials: {
      accessKeyId: ACCESS_KEY_ID,
      secretAccessKey: SECRET_ACCESS_KEY,
    },
  });
}

/** ensureBucket creates the bucket, treating "already exists" as success so lessons rerun cleanly. */
export async function ensureBucket(
  s3: S3Client,
  bucket: string,
): Promise<void> {
  try {
    await s3.send(new CreateBucketCommand({ Bucket: bucket }));
  } catch (err) {
    const name = (err as { name?: string }).name;
    if (name !== "BucketAlreadyExists" && name !== "BucketAlreadyOwnedByYou") {
      throw err;
    }
  }
}

/** A StoredObject is one listing entry: its key and the store's modification time. */
export interface StoredObject {
  key: string;
  /** Whole-second precision on SeaweedFS: two PUTs 250 ms apart list the same time. */
  lastModified: Date;
}

/** listObjects returns every object under prefix, following pagination. */
export async function listObjects(
  s3: S3Client,
  bucket: string,
  prefix: string,
): Promise<StoredObject[]> {
  const objects: StoredObject[] = [];
  let token: string | undefined;
  do {
    const page = await s3.send(
      new ListObjectsV2Command({
        Bucket: bucket,
        Prefix: prefix,
        ContinuationToken: token,
      }),
    );
    for (const obj of page.Contents ?? []) {
      if (obj.Key !== undefined && obj.LastModified !== undefined) {
        objects.push({ key: obj.Key, lastModified: obj.LastModified });
      }
    }
    token = page.NextContinuationToken;
  } while (token);
  return objects;
}

/** listKeys returns every object key under prefix. */
export async function listKeys(
  s3: S3Client,
  bucket: string,
  prefix: string,
): Promise<string[]> {
  return (await listObjects(s3, bucket, prefix)).map((o) => o.key);
}

/**
 * exists reports whether one object is present, with a HEAD request. A
 * missing key surfaces as a NotFound error (HTTP 404), which is the normal
 * answer here, not a failure.
 */
export async function exists(
  s3: S3Client,
  bucket: string,
  key: string,
): Promise<boolean> {
  try {
    await s3.send(new HeadObjectCommand({ Bucket: bucket, Key: key }));
    return true;
  } catch (err) {
    if ((err as { name?: string }).name === "NotFound") return false;
    throw err;
  }
}

/**
 * deleteKeys removes the given objects in one request per 1000 keys, the S3
 * limit for DeleteObjects. A no-op for an empty list.
 */
export async function deleteKeys(
  s3: S3Client,
  bucket: string,
  keys: string[],
): Promise<void> {
  for (let i = 0; i < keys.length; i += 1000) {
    await s3.send(
      new DeleteObjectsCommand({
        Bucket: bucket,
        Delete: { Objects: keys.slice(i, i + 1000).map((Key) => ({ Key })) },
      }),
    );
  }
}
