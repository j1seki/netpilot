import { memo, useState } from 'react';
import { Cpu } from 'lucide-react';
import { ProcessInfo } from '../api/wails';

export const ProcessIcon = memo(({ pid, executable }: Pick<ProcessInfo, 'pid' | 'executable'>) => {
  const [missing, setMissing] = useState('');
  const url = `/api/applications/icon?${new URLSearchParams({ pid: String(pid), executable })}`;
  const canLoad = /^(\/|[a-z]:[\\/]|\\\\)/i.test(executable);
  return <span className="process-icon" aria-hidden="true">
    {canLoad && missing !== url
      ? <img src={url} alt="" width={20} height={20} loading="lazy" decoding="async" onError={() => setMissing(url)} />
      : <Cpu size={15} />}
  </span>;
});
