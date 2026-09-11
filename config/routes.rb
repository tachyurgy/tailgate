Rails.application.routes.draw do
  root "feed#show"
  get  "feed",            to: "feed#show"
  get  "members",         to: "members#index"
  get  "m/:handle",       to: "members#show", as: :member
  post "m/:handle/follow",   to: "follows#create",  as: :follow_member
  delete "m/:handle/follow", to: "follows#destroy", as: :unfollow_member
  post "activities",      to: "activities#create"
  post "viewer",          to: "viewer#update", as: :viewer
  get  "inspector",       to: "inspector#show"
  get  "up", to: "rails/health#show", as: :rails_health_check
end
