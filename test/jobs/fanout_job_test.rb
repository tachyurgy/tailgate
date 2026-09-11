require "test_helper"

class FanoutJobTest < ActiveSupport::TestCase
  include ActiveJob::TestHelper

  setup do
    @author = Member.create!(handle: "author", display_name: "Author")
  end

  test "creating an activity writes an outbox event in the same transaction and enqueues fan-out" do
    assert_enqueued_with(job: FanoutJob) do
      @activity = @author.activities.create!(kind: "streak", payload: { days: 3 })
    end
    ev = @activity.outbox_events.sole
    assert_equal "activity.created", ev.event_type
    assert_nil ev.published_at
  end

  test "an outbox event is published at most once even if the job runs twice" do
    activity = @author.activities.create!(kind: "streak", payload: { days: 3 })
    FanoutJob.perform_now(activity.id)
    FanoutJob.perform_now(activity.id)
    calls = Fanout.client.calls.count { |c| c.first == :fanout }
    assert_equal 1, calls, "fanoutd was called #{calls} times for one activity"
    assert activity.reload.fanned_out_at.present?
    assert_equal "push", activity.fanout_mode
  end

  test "a failed publish keeps the event pending with the error recorded, and retries" do
    failing = Object.new
    def failing.fanout(**) = raise(Fanout::Error, "fanoutd 503")
    Fanout.client = failing
    activity = @author.activities.create!(kind: "streak", payload: { days: 3 })
    # retry_on swallows the error and schedules the next attempt; the event must stay pending.
    assert_enqueued_with(job: FanoutJob, args: [activity.id]) { FanoutJob.perform_now(activity.id) }
    ev = activity.outbox_events.sole.reload
    assert_nil ev.published_at
    assert_equal 1, ev.attempts
    assert_match(/503/, ev.last_error)
    assert_nil activity.reload.fanned_out_at
  end
end
