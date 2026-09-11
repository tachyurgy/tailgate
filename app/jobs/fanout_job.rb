# Publishes one outbox event to fanoutd. Safe to run twice: fanoutd inserts with
# ON CONFLICT DO NOTHING on (member_id, activity_id), so a retry after a timeout
# cannot deliver an activity to a follower twice.
class FanoutJob < ApplicationJob
  queue_as :default
  retry_on Fanout::Error, wait: :polynomially_longer, attempts: 8

  def perform(activity_id)
    event = OutboxEvent.find_by(activity_id: activity_id, event_type: "activity.created")
    return if event.nil? || event.published_at.present?
    event.increment!(:attempts)
    begin
      result = Fanout.client.fanout(activity_id: activity_id)
    rescue Fanout::Error => e
      event.update!(last_error: e.message.truncate(500))
      raise
    end
    Activity.where(id: activity_id).update_all(
      fanned_out_at: Time.current, fanout_mode: result[:mode], deliveries_count: result[:delivered].to_i
    )
    event.update!(published_at: Time.current, last_error: nil)
  end
end
