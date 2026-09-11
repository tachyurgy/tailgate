class Follow < ApplicationRecord
  belongs_to :follower, class_name: "Member", inverse_of: :active_follows
  belongs_to :followee, class_name: "Member", inverse_of: :passive_follows
  # Uniqueness is enforced by the database index so create_or_find_by! can race safely;
  # a model-level uniqueness validation would pre-empt that path with a SELECT.
  validate { errors.add(:followee, "cannot follow yourself") if follower_id == followee_id }
end
