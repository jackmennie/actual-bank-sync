import { mkdir, readFileSync } from 'node:fs';
import { promisify } from 'node:util';
import * as api from '@actual-app/api';

console.log = (...values) => console.error(...values);
console.info = (...values) => console.error(...values);
console.debug = (...values) => console.error(...values);

const mkdirAsync = promisify(mkdir);

function requiredString(value, name) {
  if (typeof value !== 'string' || value.trim() === '') {
    throw new Error(`${name} is required`);
  }
  return value;
}

async function run(request) {
  requiredString(request.serverURL, 'serverURL');
  requiredString(request.password, 'password');
  requiredString(request.syncId, 'syncId');
  requiredString(request.dataDir, 'dataDir');

  await mkdirAsync(request.dataDir, { recursive: true });
  await api.init({
    serverURL: request.serverURL,
    password: request.password,
    dataDir: request.dataDir,
    verbose: Boolean(request.verbose),
  });

  try {
    await api.downloadBudget(request.syncId, {
      password: request.encryptionPassword || undefined,
    });

    if (request.action === 'accounts') {
      const accounts = await api.getAccounts();
      return accounts.map(account => ({
        id: account.id,
        name: account.name,
        offbudget: account.offbudget,
        closed: account.closed,
      }));
    }

    if (request.action === 'import-transactions') {
      requiredString(request.accountId, 'accountId');
      if (!Array.isArray(request.transactions)) {
        throw new Error('transactions must be an array');
      }

      const transferTargets = new Set(
        request.transactions
          .map(transaction => transaction.transferAccountId)
          .filter(Boolean),
      );
      const payees = transferTargets.size > 0 ? await api.getPayees() : [];
      const transactions = request.transactions.map(transaction => {
        const { transferAccountId, ...importTransaction } = transaction;
        if (!transferAccountId) return importTransaction;

        const transferPayee = payees.find(
          payee => payee.transfer_acct === transferAccountId,
        );
        if (!transferPayee) {
          throw new Error(
            `Actual transfer payee not found for account ${transferAccountId}`,
          );
        }
        return { ...importTransaction, payee: transferPayee.id };
      });

      const result = await api.importTransactions(
        request.accountId,
        transactions,
        {
          defaultCleared: true,
          dryRun: Boolean(request.dryRun),
          reimportDeleted: false,
          payeeNameNormalization: 'original',
        },
      );

      if (!request.dryRun && !result.errors?.length) {
        await api.sync();
      }
      if (!request.dryRun) return result;

      const updatedByImportedId = new Map(
        (result.updatedPreview || [])
          .filter(item => item.transaction?.imported_id)
          .map(item => [item.transaction.imported_id, item]),
      );
      const preview = transactions.flatMap(transaction => {
        const matched = updatedByImportedId.get(transaction.imported_id);
        if (matched?.ignored) return [];
        if (matched?.existing && typeof matched.existing === 'object') {
          return [{
            importedId: transaction.imported_id,
            action: 'updated',
            existing: {
              date: matched.existing.date,
              amount: matched.existing.amount,
              payee_name: matched.existing.payee_name,
              notes: matched.existing.notes,
            },
          }];
        }
        return [{ importedId: transaction.imported_id, action: 'added' }];
      });
      return { ...result, preview };
    }

    throw new Error(`unsupported action: ${request.action}`);
  } finally {
    await api.shutdown();
  }
}

try {
  const request = JSON.parse(readFileSync(0, 'utf8'));
  const result = await run(request);
  process.stdout.write(`${JSON.stringify(result)}\n`);
} catch (error) {
  const message = error instanceof Error ? error.message : String(error);
  process.stderr.write(`${message}\n`);
  process.stdout.write(`${JSON.stringify({ error: message })}\n`);
  process.exitCode = 1;
}
