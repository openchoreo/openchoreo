import { ReactNode, FC, createContext, useContext } from 'react';
import { ModelsBuild, ModelsWorkload } from '@internal/plugin-openchoreo-api';

interface WorkloadContextType {
  builds: ModelsBuild[];
  workloadSpec: ModelsWorkload | null;
  setWorkloadSpec: (spec: ModelsWorkload | null) => void;
  isDeploying: boolean;
}

const WorkloadContext = createContext<WorkloadContextType | undefined>(
  undefined,
);

export const WorkloadProvider: FC<{
  builds: ModelsBuild[];
  workloadSpec: ModelsWorkload | null;
  setWorkloadSpec: (spec: ModelsWorkload | null) => void;
  children: ReactNode;
  isDeploying: boolean;
}> = ({ builds, workloadSpec, setWorkloadSpec, children, isDeploying }) => {
  return (
    <WorkloadContext.Provider
      value={{ builds, workloadSpec, setWorkloadSpec, isDeploying }}
    >
      {children}
    </WorkloadContext.Provider>
  );
};

export const useWorkloadContext = (): WorkloadContextType => {
  const context = useContext(WorkloadContext);
  if (context === undefined) {
    throw new Error(
      'useWorkloadContext must be used within a WorkloadProvider',
    );
  }
  return context;
};

export const useIsDeploying = () => {
  const { isDeploying } = useWorkloadContext();
  return isDeploying;
};

// Keep backwards compatibility
export const useBuilds = () => {
  const { builds } = useWorkloadContext();
  return { builds };
};
