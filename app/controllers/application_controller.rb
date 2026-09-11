class ApplicationController < ActionController::Base
  allow_browser versions: :modern
  helper_method :viewer

  # The demo has no login: the "viewer" is whichever member you chose to browse as.
  def viewer
    @viewer ||= Member.find_by(id: session[:viewer_id]) || Member.order(:id).first
  end
end
