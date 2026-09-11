require "test_helper"

class MemberTest < ActiveSupport::TestCase
  setup do
    @a = Member.create!(handle: "alpha", display_name: "Alpha")
    @b = Member.create!(handle: "bravo", display_name: "Bravo")
  end

  test "follow is idempotent at the edge and the counters, and tells fanoutd once" do
    @a.follow!(@b); @a.follow!(@b)
    assert_equal 1, Follow.count
    assert_equal 1, @a.reload.following_count
    assert_equal 1, @b.reload.followers_count
    assert_equal 1, Fanout.client.calls.count { |c| c.first == :follow }
  end

  test "self-follow is rejected by the database, not just the model" do
    assert_raises(ActiveRecord::StatementInvalid) { Follow.insert!({ follower_id: @a.id, followee_id: @a.id, created_at: Time.current, updated_at: Time.current }) }
  end

  test "unfollow prunes counters and tells fanoutd" do
    @a.follow!(@b); @a.unfollow!(@b); @a.unfollow!(@b)
    assert_equal 0, Follow.count
    assert_equal 0, @a.reload.following_count
    assert_equal 0, @b.reload.followers_count
    assert_equal 1, Fanout.client.calls.count { |c| c.first == :unfollow }
  end

  test "celebrity threshold flips fan-out mode" do
    @b.update!(followers_count: Member::CELEBRITY_THRESHOLD)
    assert @b.celebrity?
    assert_not @a.celebrity?
  end
end
