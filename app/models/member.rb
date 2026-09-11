class Member < ApplicationRecord
  has_many :activities, dependent: :destroy
  has_many :active_follows,  class_name: "Follow", foreign_key: :follower_id, dependent: :destroy, inverse_of: :follower
  has_many :passive_follows, class_name: "Follow", foreign_key: :followee_id, dependent: :destroy, inverse_of: :followee
  has_many :following, through: :active_follows,  source: :followee
  has_many :followers, through: :passive_follows, source: :follower

  validates :handle, presence: true, uniqueness: true, format: { with: /\A[a-z0-9_]{2,24}\z/ }
  validates :display_name, presence: true

  # Above this many followers an author is fanned out lazily (pull) instead of
  # materialised into every follower's timeline (push). The classic hybrid.
  CELEBRITY_THRESHOLD = Integer(ENV.fetch("CELEBRITY_THRESHOLD", 40))

  def celebrity? = followers_count >= CELEBRITY_THRESHOLD
  def to_param = handle
  def follows?(other) = active_follows.exists?(followee_id: other.id)

  # Follow/unfollow go through fanoutd so the timeline is backfilled/pruned atomically
  # with the edge; the edge itself is written here (Rails owns the graph).
  def follow!(other)
    return false if other == self
    created = Follow.transaction do
      edge = Follow.create_or_find_by!(follower: self, followee: other)
      if edge.previously_new_record?
        Member.increment_counter(:following_count, id)
        Member.increment_counter(:followers_count, other.id)
      end
      edge.previously_new_record?
    end
    Fanout.client.follow(follower_id: id, followee_id: other.id) if created
    created
  end

  def unfollow!(other)
    removed = Follow.transaction do
      row = active_follows.find_by(followee_id: other.id)
      if row
        row.destroy!
        Member.decrement_counter(:following_count, id)
        Member.decrement_counter(:followers_count, other.id)
      end
      !row.nil?
    end
    Fanout.client.unfollow(follower_id: id, followee_id: other.id) if removed
    removed
  end
end
