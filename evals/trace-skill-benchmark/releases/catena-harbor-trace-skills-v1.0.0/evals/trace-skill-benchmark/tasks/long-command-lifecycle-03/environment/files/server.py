import http.server,json,time
time.sleep(12)
class H(http.server.BaseHTTPRequestHandler):
 def do_GET(self):
  body=json.dumps({"state":"ready","service":"delta"}).encode();self.send_response(200);self.end_headers();self.wfile.write(body)
 def log_message(self,*args): pass
http.server.HTTPServer(("127.0.0.1",8571),H).serve_forever()
