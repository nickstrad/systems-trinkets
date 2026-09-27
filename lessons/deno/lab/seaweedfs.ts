// Module seaweedfs holds the S3 helpers object-store lessons repeat. It lives
// beside lab.ts rather than in it so a lesson that never talks to SeaweedFS
// does not load the AWS SDK.
import {
  CreateBucketCommand,
  DeleteObjectsCommand,
  ListObjectsV2Command,
  S3Client,
} from "@aws-sdk/client-s3";
import { env } from "lab/lab.ts";

/**
 * DEFAULT_URL is the local dev S3 gateway from services/index.md. The
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

/** listKeys returns every object key under prefix, following pagination. */
export async function listKeys(
  s3: S3Client,
  bucket: string,
  prefix: string,
): Promise<string[]> {
  const keys: string[] = [];
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
      if (obj.Key !== undefined) keys.push(obj.Key);
    }
    token = page.NextContinuationToken;
  } while (token);
  return keys;
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
