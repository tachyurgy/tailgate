class MembersController < ApplicationController
  def index
    @members = Member.order(followers_count: :desc, handle: :asc)
  end

  def show
    @member = Member.find_by!(handle: params[:handle])
    @activities = @member.activities.order(created_at: :desc).limit(30)
    @following = viewer.follows?(@member)
  end
end
