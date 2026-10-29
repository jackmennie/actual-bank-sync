SHELL=/bin/sh
PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin

# Import settled UP transactions every 15 minutes.
*/15 * * * * actual-bank-sync /usr/bin/flock -n /var/lib/actual-bank-sync/cron.lock /usr/bin/actual-bank-sync --config /etc/actual-bank-sync/config.yaml sync >> /var/log/actual-bank-sync/sync.log 2>&1