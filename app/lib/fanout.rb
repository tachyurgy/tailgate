require "net/http"
require "json"

# HTTP client for fanoutd, the Go service that owns timeline_entries.
# Every call is idempotent on the server, so the caller may retry freely.
module Fanout
  class Error < StandardError; end

  class Client
    def initialize(base = ENV.fetch("FANOUTD_URL", "http://localhost:8080"))
      @base = URI(base)
    end

    def fanout(activity_id:)               = post("/fanout",   activity_id: activity_id)
    def follow(follower_id:, followee_id:)   = post("/follow",   follower_id: follower_id, followee_id: followee_id)
    def unfollow(follower_id:, followee_id:) = post("/unfollow", follower_id: follower_id, followee_id: followee_id)

    # Returns [{activity_id:, author_id:, created_at:, source: "push"|"pull"}], newest first.
    def timeline(member_id:, before: nil, limit: 30)
      q = { limit: limit }
      q[:before] = before if before
      get("/timeline/#{member_id}", q)
    end

    def healthy? = get("/healthz")["ok"] == true rescue false

    private

    def post(path, body)
      req = Net::HTTP::Post.new(path, "Content-Type" => "application/json")
      req.body = body.to_json
      run(req)
    end

    def get(path, query = {})
      uri = @base.dup; uri.path = path; uri.query = URI.encode_www_form(query) unless query.empty?
      run(Net::HTTP::Get.new(uri))
    end

    def run(req)
      res = Net::HTTP.start(@base.host, @base.port, read_timeout: 5, open_timeout: 2) { |h| h.request(req) }
      raise Error, "fanoutd #{req.path} -> #{res.code} #{res.body.to_s[0, 200]}" unless res.is_a?(Net::HTTPSuccess)
      JSON.parse(res.body, symbolize_names: true)
    end
  end

  class NullClient
    # Used in tests that do not run fanoutd. Records calls so tests can assert on them.
    attr_reader :calls
    def initialize = @calls = []
    def fanout(**a)   = (@calls << [:fanout, a];   { ok: true, delivered: 0, mode: "push", already: false })
    def follow(**a)   = (@calls << [:follow, a];   { ok: true })
    def unfollow(**a) = (@calls << [:unfollow, a]; { ok: true })
    def timeline(**)  = []
    def healthy?      = true
  end

  def self.client
    @client ||= Client.new
  end

  def self.client=(c)
    @client = c
  end
end
