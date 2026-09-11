class FeedController < ApplicationController
  def show
    @before = params[:before].presence
    @rows = Fanout.client.timeline(member_id: viewer.id, before: @before, limit: 25)
    acts = Activity.includes(:member).where(id: @rows.map { _1[:activity_id] }).index_by(&:id)
    @items = @rows.filter_map { |r| a = acts[r[:activity_id]] and [a, r[:source]] }
    @next_before = @rows.last&.dig(:cursor)
    @healthy = true
  rescue Fanout::Error, Errno::ECONNREFUSED => e
    @items, @healthy, @error = [], false, e.message
  end
end
