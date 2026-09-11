class ViewerController < ApplicationController
  def update
    session[:viewer_id] = Member.find_by!(handle: params[:handle]).id
    redirect_back fallback_location: root_path
  end
end
