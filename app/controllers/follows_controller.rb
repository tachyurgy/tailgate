class FollowsController < ApplicationController
  def create
    m = Member.find_by!(handle: params[:handle])
    viewer.follow!(m)
    redirect_to member_path(m), notice: "Following @#{m.handle}. Their recent activity was backfilled into your feed."
  end

  def destroy
    m = Member.find_by!(handle: params[:handle])
    viewer.unfollow!(m)
    redirect_to member_path(m), notice: "Unfollowed @#{m.handle}. Their activity was removed from your feed."
  end
end
