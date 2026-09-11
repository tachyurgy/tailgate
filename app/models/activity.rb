class Activity < ApplicationRecord
  KINDS = %w[pick_placed entry_won streak milestone].freeze
  belongs_to :member
  attribute :payload, :jsonb, default: -> { {} }
  has_many :outbox_events, dependent: :destroy
  validates :kind, inclusion: { in: KINDS }

  # Creating an activity and queueing its fan-out happen in ONE transaction (the outbox
  # pattern), so a crash between "row written" and "job enqueued" cannot strand a post.
  after_create_commit :enqueue_fanout
  after_create { outbox_events.create!(event_type: "activity.created") }   # same transaction as the activity row

  def author = member

  def summary
    case kind
    when "pick_placed" then "placed a #{payload['picks']}-pick #{payload['mode']} on #{payload['league']}"
    when "entry_won"   then "won a #{payload['picks']}-pick #{payload['mode']} (#{payload['multiplier']}x) on #{payload['league']}"
    when "streak"      then "is on a #{payload['days']}-day streak"
    when "milestone"   then payload["text"].to_s
    end
  end

  private

  def enqueue_fanout
    FanoutJob.perform_later(id)
  end
end
