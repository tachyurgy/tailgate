class OutboxEvent < ApplicationRecord
  belongs_to :activity
  scope :pending, -> { where(published_at: nil) }
end
