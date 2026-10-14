#!/usr/bin/env python3
"""Minimal in-memory S3 stub for scripts/test-dj-api.sh.

Implements just enough of the S3 API for the KlubHub DJ API binary
(minio-go client) to boot and serve against a disposable stack:

  GET    /{bucket}?location=   -> LocationConstraint XML (region lookup)
  HEAD   /{bucket}             -> 200 (bucket exists)
  PUT    /{bucket}             -> 200 (MakeBucket)
  PUT    /{bucket}/{object}    -> 200 + ETag (PutObject)
  GET    /{bucket}/{object}    -> 200 stored bytes / 404 NoSuchKey
  HEAD   /{bucket}/{object}    -> 200 / 404 (StatObject)
  DELETE /{bucket}/{object}    -> 204 (RemoveObject)

Objects live in an in-memory dict; nothing touches disk. Signatures are
not validated — the stub exists so the API can be exercised end-to-end
without a real Garage bucket.

Usage: test-dj-api-s3-stub.py <port>
"""
import sys
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import urlparse, parse_qs

# keep_blank_values: the bucket-location query is literally `?location=`
# (blank value); the default parse_qs drops it, which made the stub answer
# with an empty body and minio-go's XML decoder fail with EOF.
_QS = {"keep_blank_values": True}

OBJECTS = {}

LOCATION_XML = (
    '<?xml version="1.0" encoding="UTF-8"?>'
    '<LocationConstraint xmlns="http://s3.amazonaws.com/doc/2006-03-01/">'
    'us-east-1</LocationConstraint>'
)

NO_SUCH_KEY = (
    '<?xml version="1.0" encoding="UTF-8"?>'
    '<Error><Code>NoSuchKey</Code><Message>not found</Message></Error>'
)


def _decode_streaming(raw):
    data = b""
    pos = 0
    while pos < len(raw):
        eol = raw.find(b"\r\n", pos)
        if eol < 0:
            break
        size = int(raw[pos:eol].split(b";")[0], 16)
        pos = eol + 2
        if size == 0:
            break
        data += raw[pos:pos + size]
        pos += size + 2  # data + trailing CRLF
    return data


class Handler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, fmt, *args):
        sys.stderr.write("s3-stub: " + (fmt % args) + "\n")

    def _split(self):
        u = urlparse(self.path)
        parts = [p for p in u.path.split("/") if p]
        bucket = parts[0] if parts else ""
        obj = "/".join(parts[1:])
        return bucket, obj, parse_qs(u.query, **_QS)

    def _read_body(self):
        if "chunked" in self.headers.get("Transfer-Encoding", ""):
            data = b""
            while True:
                size = int(self.rfile.readline().strip(), 16)
                if size == 0:
                    while True:
                        line = self.rfile.readline()
                        if line in (b"\r\n", b"\n", b""):
                            break
                    break
                data += self.rfile.read(size)
                self.rfile.read(2)  # trailing CRLF
            return data
        length = int(self.headers.get("Content-Length") or 0)
        raw = self.rfile.read(length) if length else b""
        if "STREAMING" in self.headers.get("X-Amz-Content-Sha256", ""):
            # minio-go signs PUT bodies over plain HTTP with
            # STREAMING-AWS4-HMAC-SHA256-PAYLOAD: the wire body is
            # <hex-len>;chunk-signature=<sig>\r\n<data>\r\n ... 0;chunk-signature=<sig>\r\n\r\n
            # Strip the framing to recover the actual object bytes.
            return _decode_streaming(raw)
        return raw

    def _send(self, code, body=b"", content_length=None, extra=None):
        self.send_response(code)
        self.send_header("Content-Length", str(len(body) if content_length is None else content_length))
        self.send_header("Content-Type", "application/xml")
        for k, v in (extra or {}).items():
            self.send_header(k, v)
        self.end_headers()
        if body and self.command != "HEAD":
            self.wfile.write(body)

    def do_GET(self):
        _, obj, qs = self._split()
        if obj == "" and "location" in qs:
            self._send(200, LOCATION_XML.encode())
            return
        if obj == "":
            self._send(200, b"")
            return
        if obj in OBJECTS:
            # minio-go parses Last-Modified on GET responses too; without
            # it Object.Read fails with "Last-Modified time format is invalid".
            self._send(200, OBJECTS[obj], extra={
                "Content-Type": "application/octet-stream",
                "Last-Modified": "Wed, 14 Oct 2026 00:00:00 GMT",
                "ETag": '"dj-api-test-stub"',
            })
        else:
            self._send(404, NO_SUCH_KEY.encode())

    def do_HEAD(self):
        _, obj, _ = self._split()
        if obj == "":
            self._send(200)
        elif obj in OBJECTS:
            # minio-go's StatObject parses Last-Modified; without it the
            # call fails with "Last-Modified time format is invalid".
            self._send(200, content_length=len(OBJECTS[obj]), extra={
                "Last-Modified": "Wed, 14 Oct 2026 00:00:00 GMT",
                "ETag": '"dj-api-test-stub"',
            })
        else:
            self._send(404)

    def do_PUT(self):
        _, obj, _ = self._split()
        body = self._read_body()
        if obj == "":
            self._send(200)  # MakeBucket
            return
        OBJECTS[obj] = body
        self._send(200, extra={"ETag": '"dj-api-test-stub"'})

    def do_DELETE(self):
        _, obj, _ = self._split()
        OBJECTS.pop(obj, None)
        self._send(204)


if __name__ == "__main__":
    port = int(sys.argv[1])
    ThreadingHTTPServer(("127.0.0.1", port), Handler).serve_forever()
