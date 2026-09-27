// Module lab holds the few helpers every Deno lesson repeats: environment
// defaults and the measurements.csv writer. It mirrors Go's internal/lab and,
// like it, imports no driver, so a lesson only loads the clients it uses.
// Driver helpers, including each service's URL, live beside it in
// lab/postgres.ts, lab/valkey.ts, and lab/seaweedfs.ts.

/** env returns the environment variable named key, or def when it is unset or empty. */
export function env(key: string, def: string): string {
  return Deno.env.get(key) || def;
}

/**
 * Measurements is the CSV file a lesson writes and its analyze.sql reads.
 * Rows go straight to the file, so a crash mid-run leaves a partial file
 * rather than nothing.
 */
export class Measurements {
  #file: Deno.FsFile;
  #encoder = new TextEncoder();

  /**
   * Creates measurements.csv in the current directory and writes the header
   * row. Lessons run from their own directory, which is where analyze.sql
   * expects the file.
   */
  constructor(...columns: string[]) {
    this.#file = Deno.openSync("measurements.csv", {
      write: true,
      create: true,
      truncate: true,
    });
    this.write(...columns);
  }

  /**
   * Appends one row. Numbers are written with String(), so pass a string
   * (for example n.toFixed(3)) when the lesson wants a specific precision.
   */
  write(...fields: (string | number)[]): void {
    const line = fields.map((f) => quote(String(f))).join(",") + "\n";
    this.#file.writeSync(this.#encoder.encode(line));
  }

  /** Closes the file. */
  close(): void {
    this.#file.close();
    console.log("wrote measurements.csv");
  }
}

// quote wraps a field in double quotes when it contains a comma, quote, or
// newline, the way encoding/csv does in Go.
function quote(field: string): string {
  return /[",\n\r]/.test(field) ? `"${field.replaceAll('"', '""')}"` : field;
}
