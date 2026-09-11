# Read-only view of the table fanoutd owns. Rails never writes it.
class TimelineEntry < ApplicationRecord
  belongs_to :activity
  belongs_to :member
  def readonly? = true
end
