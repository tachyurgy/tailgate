class InspectorController < ApplicationController
  def show
    @pending  = OutboxEvent.pending.includes(activity: :member).order(:created_at).limit(50)
    @recent   = Activity.includes(:member).order(created_at: :desc).limit(40)
    @celebs   = Member.where("followers_count >= ?", Member::CELEBRITY_THRESHOLD).order(followers_count: :desc)
    @entries  = TimelineEntry.count
    @healthy  = Fanout.client.healthy?
  end
end
