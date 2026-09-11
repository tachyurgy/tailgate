class ActivitiesController < ApplicationController
  LEAGUES = %w[NFL NBA MLB NHL LoL CS2].freeze

  def create
    kind = params[:kind].presence_in(Activity::KINDS) || "pick_placed"
    payload = case kind
              when "pick_placed" then { picks: rand(2..6), mode: %w[Power Flex].sample, league: LEAGUES.sample }
              when "entry_won"   then { picks: rand(2..6), mode: %w[Power Flex].sample, league: LEAGUES.sample, multiplier: [3, 5, 10, 25].sample }
              when "streak"      then { days: rand(3..30) }
              else { text: params[:text].to_s.truncate(140) }
              end
    a = viewer.activities.create!(kind: kind, payload: payload)
    redirect_to member_path(viewer), notice: "Posted. Fan-out queued as outbox event ##{a.outbox_events.first.id}."
  end
end
