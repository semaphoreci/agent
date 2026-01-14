FROM ruby:3.3

WORKDIR /app

CMD bundle config set path 'vendor/bundle' && bundle install && bundle exec ruby app.rb
