FROM ruby:2.7

WORKDIR /app

CMD bundle config set path 'vendor/bundle' && bundle install && bundle exec ruby app.rb
